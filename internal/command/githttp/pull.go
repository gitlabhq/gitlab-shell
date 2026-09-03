// Package githttp provides functionality to handle Git operations over HTTP(S) and SSH,
// including executing Git commands like git-upload-pack and converting responses to the
// expected format for SSH protocols. It integrates with GitLab's internal components
// for secure access verification and data transfer.
package githttp

import (
	"context"
	"io"
	"log/slog"

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

	return pipeRequest(ctx, c.ReadWriter, c.readFromStdin, client.SSHUploadPack)
}

func (c *PullCommand) requestUploadPack(ctx context.Context, client *git.Client) error {
	return pipeRequest(ctx, c.ReadWriter, c.readFromStdin, client.UploadPack)
}

// readFromStdin forwards pkt-lines from stdin until it sees `done`.
//
// Fix for https://gitlab.com/gitlab-org/gitlab/-/work_items/584782: it closes
// pw once negotiation ends instead of keeping it open (and thus subject to
// the primary's nginx client_body_timeout) for the whole pack transfer, which
// only flows on the response side.
func (c *PullCommand) readFromStdin(pw *io.PipeWriter) {
	scanner := pktline.NewScanner(c.ReadWriter.In)

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
