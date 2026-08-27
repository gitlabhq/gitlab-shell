// Package githttp provides functionality to handle Git operations over HTTP(S) and SSH,
// including executing Git commands like git-upload-pack and converting responses to the
// expected format for SSH protocols. It integrates with GitLab's internal components
// for secure access verification and data transfer.
package githttp

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/git"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/pktline"
	"gitlab.com/gitlab-org/labkit/v2/log"
)

const pullService = "git-upload-pack"

var uploadPackHTTPPrefix = []byte("001e# service=git-upload-pack\n0000")

// PullCommand handles the execution of a Git pull operation over HTTP(S) or SSH
type PullCommand struct {
	Config     *config.Config
	ReadWriter *readwriter.ReadWriter
	Args       *commandargs.Shell
	Response   *accessverifier.Response
}

// See Uploading Data > HTTP(S) section at:
// https://git-scm.com/book/en/v2/Git-Internals-Transfer-Protocols
//
// 1. Perform /info/refs?service=git-upload-pack request
// 2. Remove the header to make it consumable by SSH protocol
// 3. Send the result to the user via SSH (writeToStdout)
// 4. Read the send-pack data provided by user via SSH (stdinReader)
// 5. Perform /git-upload-pack request and send this data
// 6. Return the output to the user

// ForInfoRefs returns the necessary Pull specifics for client.InfoRefs()
func (c *PullCommand) ForInfoRefs() (*readwriter.ReadWriter, string, []byte) {
	return c.ReadWriter, pullService, uploadPackHTTPPrefix
}

// Execute runs the pull command by determining the appropriate method (HTTP/SSH)
func (c *PullCommand) Execute(ctx context.Context) error {
	data := c.Response.Payload.Data
	client := &git.Client{URL: data.PrimaryRepo, Headers: data.RequestHeaders}

	// For Git over SSH routing
	if data.GeoProxyFetchSSHDirectToPrimary {
		client.Headers["Git-Protocol"] = c.Args.Env.GitProtocolVersion
		return c.requestSSHUploadPack(ctx, client)
	}

	if err := requestInfoRefs(ctx, client, c); err != nil {
		return err
	}

	return c.requestUploadPack(ctx, client)
}

func (c *PullCommand) requestSSHUploadPack(ctx context.Context, client *git.Client) error {
	slog.InfoContext(ctx, "Using Git over SSH upload pack")

	// The SSH path maps a whole negotiation (potentially several rounds) onto
	// one full-duplex HTTP exchange (see workhorse's EnableFullDuplex for this
	// endpoint), so it's the one that needs the "ready" response-side signal:
	// see copyUploadPackResponse.
	return c.pipeUploadPack(ctx, client.SSHUploadPack, true)
}

func (c *PullCommand) requestUploadPack(ctx context.Context, client *git.Client) error {
	// The plain HTTP path already does one round of negotiation per request
	// (the client itself issues a new POST per round), so its request body is
	// always short-lived and closing on `done` (or real EOF) is enough.
	return c.pipeUploadPack(ctx, client.UploadPack, false)
}

// pipeUploadPack streams the client's stdin into an upload-pack request via
// requestFn, then copies the response back to stdout.
//
// Fix for https://gitlab.com/gitlab-org/gitlab/-/work_items/584782:
// the request body is closed once negotiation ends instead of staying open
// (and thus subject to the primary's nginx client_body_timeout) for the
// whole pack transfer, which only flows on the response side.
func (c *PullCommand) pipeUploadPack(ctx context.Context, requestFn func(context.Context, io.Reader) (*http.Response, error), closeOnReady bool) error {
	pipeReader, pipeWriter := io.Pipe()
	requestClosed := make(chan struct{})
	go c.readFromStdin(pipeWriter, requestClosed)

	response, err := requestFn(ctx, pipeReader)
	if err != nil {
		return err
	}
	defer response.Body.Close() //nolint:errcheck

	if !closeOnReady {
		_, err = io.Copy(c.ReadWriter.Out, response.Body)
		return err
	}

	return c.copyUploadPackResponse(pipeWriter, requestClosed, response.Body)
}

// readFromStdin closes requestClosed when it returns, which is always after
// pw has been closed - either by itself (below) or, if copyUploadPackResponse
// closed it first, by the resulting write error.
func (c *PullCommand) readFromStdin(pw *io.PipeWriter, requestClosed chan struct{}) {
	defer close(requestClosed)

	scanner := pktline.NewScanner(c.ReadWriter.In)

	for scanner.Scan() {
		line := scanner.Bytes()

		if _, err := pw.Write(line); err != nil {
			if errors.Is(err, io.ErrClosedPipe) {
				// Expected once the response side has already closed pw (see
				// copyUploadPackResponse); nothing more to forward then.
				slog.Debug("stopped forwarding stdin: request body already closed", log.ErrorMessage(err.Error()))
			} else {
				slog.Error("failed to write line", log.ErrorMessage(err.Error()))
			}
			break
		}

		if pktline.IsDone(line) {
			// Inject a flush packet: protocol v2 needs one to terminate the request,
			// and upload-pack dies on EOF without it. We don't read one from the
			// client because v0/v1 send no flush after done, which would block.
			if _, err := pw.Write(pktline.PktFlush()); err != nil {
				slog.Error("failed to write flush packet", log.ErrorMessage(err.Error()))
			}

			break
		}
	}

	// scanner.Err() is non-nil if stdin was truncated mid-pkt-line rather
	// than ending cleanly; closing with that error (instead of a plain
	// Close()) means the primary sees a broken request instead of one that
	// looks complete, so upload-pack fails with something diagnosable
	// instead of a confusing downstream error. CloseWithError never itself
	// returns an error, so there's nothing to log here.
	_ = pw.CloseWithError(scanner.Err())
}

// copyUploadPackResponse copies a v2 upload-pack response to stdout while
// watching for the "ready" packet. A v2 fetch can legitimately end its
// negotiation with a flush-pkt instead of `done` (e.g. an incremental fetch
// with real history), in which case readFromStdin has nothing to close on
// and the request body would otherwise stay open for the whole pack
// transfer - reproducing https://gitlab.com/gitlab-org/gitlab/-/work_items/584782
// for that shape. "ready" is the server's unambiguous signal that
// negotiation is over and the packfile follows, so closing the request body
// there is always safe, including across multiple negotiation rounds: absent
// "ready", the response only ever contains acknowledgments asking for more
// `have`s, never a packfile.
//
// requestClosed lets this stop watching as soon as the request side has
// already closed on its own (the common case: `done`, or an ls-refs-only
// request with no fetch at all), so a long pack transfer isn't parsed
// pkt-line by pkt-line for no reason.
func (c *PullCommand) copyUploadPackResponse(pw *io.PipeWriter, requestClosed <-chan struct{}, body io.Reader) error {
	br := bufio.NewReader(body)

	for {
		select {
		case <-requestClosed:
			_, err := io.Copy(c.ReadWriter.Out, br)
			return err
		default:
		}

		pkt, err := pktline.ReadPkt(br)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			if errors.Is(err, pktline.ErrNotPktLine) {
				// Not actually pkt-line framed after all - e.g. a plain-text
				// error appended past an already-committed response. br still
				// has these bytes unconsumed, so forward them verbatim
				// instead of losing them behind a decode error.
				_, copyErr := io.Copy(c.ReadWriter.Out, br)
				return copyErr
			}
			return err
		}

		if _, err := c.ReadWriter.Out.Write(pkt); err != nil {
			return err
		}

		if pktline.IsReady(pkt) {
			if err := pw.Close(); err != nil {
				slog.Error("failed to close writer", log.ErrorMessage(err.Error()))
			}

			_, err := io.Copy(c.ReadWriter.Out, br)
			return err
		}
	}
}
