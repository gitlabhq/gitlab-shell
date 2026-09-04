package githttp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/git"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/pktline"
	"gitlab.com/gitlab-org/labkit/v2/log"
)

type gitHTTPCommand interface {
	ForInfoRefs() (*readwriter.ReadWriter, string, []byte)
}

// requestInfoRefs performs an HTTP request to the /info/refs endpoint for the specified Git service,
// verifies the response prefix, and writes the result to the output stream.
func requestInfoRefs(ctx context.Context, client *git.Client, command gitHTTPCommand) error {
	readWriter, serviceName, httpPrefix := command.ForInfoRefs()

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
	_, err = response.Body.Read(p)
	if err != nil || !bytes.Equal(p, httpPrefix) {
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

// pipeRequest streams stdin into requestFn's body via writeFn, then copies the response to rw.Out.
func pipeRequest(ctx context.Context, rw *readwriter.ReadWriter, writeFn func(io.Reader, *io.PipeWriter), requestFn func(context.Context, io.Reader) (*http.Response, error)) error {
	pipeReader, pipeWriter := io.Pipe()
	go writeFn(rw.In, pipeWriter)

	response, err := requestFn(ctx, pipeReader)
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
func readUploadPackRequest(in io.Reader, pw *io.PipeWriter) {
	scanner := pktline.NewScanner(in)

	for scanner.Scan() {
		line := scanner.Bytes()

		_, err := pw.Write(line)
		if err != nil {
			slog.Error("failed to write line", log.ErrorMessage(err.Error()))
		}

		if pktline.IsDone(line) {
			// The SSH client's request ends at `done`, and we stop reading stdin
			// here so pw can be closed (see the function comment). But the
			// primary's upload-pack is reached over HTTP, where protocol v2 expects
			// the request body to end with a flush packet after `done`; without
			// it, upload-pack sees an unexpected EOF and aborts the fetch.
			//
			// We synthesize the flush rather than forwarding the client's, because
			// protocol v0/v1 clients never send one after `done`, so waiting to
			// read it would block forever. Writing it unconditionally is safe for
			// every protocol version.
			if _, err := pw.Write(pktline.PktFlush()); err != nil {
				slog.Error("failed to write flush packet", log.ErrorMessage(err.Error()))
			}

			break
		}
	}

	err := pw.Close()
	if err != nil {
		slog.Error("failed to close writer", log.ErrorMessage(err.Error()))
	}
}

func readReceivePackRequest(in io.Reader, pw *io.PipeWriter) {
	var needsPackData bool

	scanner := pktline.NewScanner(in)
	for scanner.Scan() {
		line := scanner.Bytes()
		_, err := pw.Write(line)
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
		_, err := io.Copy(pw, in)
		if err != nil {
			slog.Error("failed to copy", log.ErrorMessage(err.Error()))
		}
	}

	err := pw.Close()
	if err != nil {
		slog.Error("failed to close writer", log.ErrorMessage(err.Error()))
	}
}
