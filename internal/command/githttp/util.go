package githttp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"time"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/git"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/pktline"
	"gitlab.com/gitlab-org/labkit/v2/log"
)

const gitProtocolHeader = "Git-Protocol"

// withGitProtocolHeader returns a copy so the access-verifier response map is never mutated.
// It sets Git-Protocol even for an empty version to preserve existing behavior.
func withGitProtocolHeader(headers map[string]string, version string) map[string]string {
	result := make(map[string]string, len(headers)+1)
	maps.Copy(result, headers)
	result[gitProtocolHeader] = version
	return result
}

type gitHTTPCommand interface {
	ForInfoRefs() (*readwriter.ReadWriter, string, []byte, time.Duration)
}

type infoRefsClient interface {
	InfoRefs(context.Context, string) (*http.Response, error)
}

// requestInfoRefs performs an HTTP request to the /info/refs endpoint for the specified Git service,
// verifies the response prefix, and writes the result to the output stream.
func requestInfoRefs(ctx context.Context, client infoRefsClient, command gitHTTPCommand) error {
	readWriter, serviceName, httpPrefix, timeout := command.ForInfoRefs()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	timer := time.AfterFunc(timeout, cancel)
	defer timer.Stop()

	response, err := client.InfoRefs(ctx, serviceName)
	if err != nil {
		return err
	}
	defer response.Body.Close() //nolint:errcheck

	// Read the first bytes that contain for
	// push - 001f# service=git-receive-pack\n0000 string
	// pull - 001e# service=git-upload-pack\n0000 string
	// to convert HTTP(S) Git response to the one expected by SSH
	p := make([]byte, len(httpPrefix))
	_, err = io.ReadFull(response.Body, p)
	if err != nil || !bytes.Equal(p, httpPrefix) {
		return fmt.Errorf("unexpected %s response", serviceName)
	}

	// An expired timer may still be canceling the request even if the prefix read succeeded.
	if !timer.Stop() {
		return fmt.Errorf("unexpected %s response", serviceName)
	}

	_, err = io.Copy(readWriter.Out, response.Body)

	return err
}

// sshRequestFunc is a function type for SSH pack requests
type sshRequestFunc func(ctx context.Context, body io.Reader) (*http.Response, error)

// executeSSHRequest executes an SSH request and copies the response to the output
func executeSSHRequest(ctx context.Context, requestFn sshRequestFunc, rw *readwriter.ReadWriter) error {
	response, err := requestFn(ctx, io.NopCloser(rw.In))
	if err != nil {
		return err
	}
	defer response.Body.Close() //nolint:errcheck

	_, err = io.Copy(rw.Out, response.Body)

	return err
}

// bodyWriter is the write side of a request body. *git.ReplayBody and
// *io.PipeWriter both satisfy it.
type bodyWriter interface {
	io.WriteCloser
	CloseWithError(err error) error
}

// startReplayBody creates a replayable request body fed by fill from in.
//
// The body is a git.ReplayBody rather than an io.Pipe so the transport can
// resend it on a fresh connection when a pooled one is lost before the server
// handled the request. With a plain pipe those retries failed with
// "net/http: cannot rewind body after connection loss".
//
// The first reader is taken before fill starts, so backpressure applies from
// the first Write. Callers must defer body.Abort(): the fill goroutine may
// still be reading stdin when the response ends, and Abort makes its next
// Write fail instead of blocking forever.
func startReplayBody(in io.Reader, fill func(io.Reader, bodyWriter)) (*git.ReplayBody, io.ReadCloser) {
	body := git.NewReplayBody(git.DefaultReplayLimit)
	reader := body.Reader()
	go fill(in, body)

	return body, reader
}

// copyRequestBody forwards in to w unchanged, passing any read error through.
func copyRequestBody(in io.Reader, w bodyWriter) {
	_, err := io.Copy(w, in)
	_ = w.CloseWithError(err)
}

// pipeRequest streams stdin into requestFn's body via writeFn, then copies the response to rw.Out.
func pipeRequest(ctx context.Context, rw *readwriter.ReadWriter, writeFn func(io.Reader, bodyWriter), requestFn func(context.Context, io.Reader) (*http.Response, error)) error {
	body, reader := startReplayBody(rw.In, writeFn)
	defer body.Abort()

	response, err := requestFn(ctx, reader)
	if err != nil {
		return err
	}
	defer response.Body.Close() //nolint:errcheck

	_, err = io.Copy(rw.Out, response.Body)

	return err
}

// readUploadPackRequest forwards pkt-lines from an io.Reader until it sees
// `done`, then closes the writer.
//
// Fix for https://gitlab.com/gitlab-org/gitlab/-/work_items/584782: it closes
// the request body once negotiation ends instead of leaving it open, subject to
// the primary's nginx client_body_timeout, for the whole pack transfer, which
// only flows on the response side. Requests that never send `done`, such as
// protocol v2 ls-refs, fall through to stdin EOF, which also closes the writer.
func readUploadPackRequest(in io.Reader, w bodyWriter) {
	scanner := pktline.NewScanner(in)

	for scanner.Scan() {
		line := scanner.Bytes()

		_, err := w.Write(line)
		if err != nil {
			slog.Error("failed to write line", log.ErrorMessage(err.Error()))
		}

		if pktline.IsDone(line) {
			// The SSH client's request ends at `done`, and we stop reading stdin
			// here so w can be closed (see the function comment). But the
			// primary's upload-pack is reached over HTTP, where protocol v2 expects
			// the request body to end with a flush packet after `done`; without
			// it, upload-pack sees an unexpected EOF and aborts the fetch.
			//
			// We synthesize the flush rather than forwarding the client's, because
			// protocol v0/v1 clients never send one after `done`, so waiting to
			// read it would block forever. Writing it unconditionally is safe for
			// every protocol version.
			if _, err := w.Write(pktline.PktFlush()); err != nil {
				slog.Error("failed to write flush packet", log.ErrorMessage(err.Error()))
			}

			break
		}
	}

	err := w.Close()
	if err != nil {
		slog.Error("failed to close writer", log.ErrorMessage(err.Error()))
	}
}

func readReceivePackRequest(in io.Reader, w bodyWriter) {
	var needsPackData bool

	scanner := pktline.NewScanner(in)
	for scanner.Scan() {
		line := scanner.Bytes()
		_, err := w.Write(line)
		if err != nil {
			slog.Error("failed to write line", log.ErrorMessage(err.Error()))
		}

		if pktline.IsFlush(line) {
			break
		}

		if !needsPackData && !pktline.IsRefRemoval(line) {
			needsPackData = true
		}
	}

	if needsPackData {
		_, err := io.Copy(w, in)
		if err != nil {
			slog.Error("failed to copy", log.ErrorMessage(err.Error()))
		}
	}

	err := w.Close()
	if err != nil {
		slog.Error("failed to close writer", log.ErrorMessage(err.Error()))
	}
}
