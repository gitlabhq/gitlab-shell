package githttp

import (
	"context"
	"io"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/git"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/pktline"
	"gitlab.com/gitlab-org/labkit/log"
)

const pushService = "git-receive-pack"

var receivePackHTTPPrefix = []byte("001f# service=git-receive-pack\n0000")

// PushCommand handles the execution of a Git push operation,
// including configuration, input/output handling, and access verification.
type PushCommand struct {
	Config     *config.Config
	ReadWriter *readwriter.ReadWriter
	Response   *accessverifier.Response
	Args       *commandargs.Shell
}

// See Uploading Data > HTTP(S) section at:
// https://git-scm.com/book/en/v2/Git-Internals-Transfer-Protocols
//
// 1. Perform /info/refs?service=git-receive-pack request
// 2. Remove the header to make it consumable by SSH protocol
// 3. Send the result to the user via SSH (writeToStdout)
// 4. Read the send-pack data provided by user via SSH (stdinReader)
// 5. Perform /git-receive-pack request and send this data
// 6. Return the output to the user

// ForInfoRefs returns the necessary Push specifics for client.InfoRefs()
func (c *PushCommand) ForInfoRefs() (*readwriter.ReadWriter, string, []byte) {
	return c.ReadWriter, pushService, receivePackHTTPPrefix
}

// Execute runs the push command by determining the appropriate method (HTTP/SSH)
func (c *PushCommand) Execute(ctx context.Context) error {
	data := c.Response.Payload.Data
	client := &git.Client{URL: data.PrimaryRepo, Headers: data.RequestHeaders}

	// For Git over SSH routing
	if data.GeoProxyPushSSHDirectToPrimary {
		client.Headers["Git-Protocol"] = c.Args.Env.GitProtocolVersion
		return c.requestSSHReceivePack(ctx, client)
	}

	if err := requestInfoRefs(ctx, client, c); err != nil {
		return err
	}

	return c.requestReceivePack(ctx, client)
}

func (c *PushCommand) requestSSHReceivePack(ctx context.Context, client *git.Client) error {
	log.ContextLogger(ctx).Info("Using Git over SSH receive pack")

	return executeSSHRequest(ctx, client.SSHReceivePack, c.ReadWriter)
}

func (c *PushCommand) requestReceivePack(ctx context.Context, client *git.Client) error {
	pipeReader, pipeWriter := io.Pipe()
	go c.readFromStdin(pipeWriter)

	response, err := client.ReceivePack(ctx, pipeReader)
	if err != nil {
		return err
	}
	defer response.Body.Close() //nolint:errcheck

	_, err = io.Copy(c.ReadWriter.Out, response.Body)

	return err
}

func (c *PushCommand) readFromStdin(pw *io.PipeWriter) {
	var needsPackData bool

	scanner := pktline.NewScanner(c.ReadWriter.In)
	for scanner.Scan() {
		line := scanner.Bytes()
		_, err := pw.Write(line)
		if err != nil {
			log.WithError(err).Error("failed to write line")
		}

		if pktline.IsFlush(line) {
			break
		}

		if !needsPackData && !pktline.IsRefRemoval(line) {
			needsPackData = true
		}
	}

	if needsPackData {
		_, err := io.Copy(pw, c.ReadWriter.In)
		if err != nil {
			log.WithError(err).Error("failed to copy")
		}
	}

	err := pw.Close()
	if err != nil {
		log.WithError(err).Error("failed to close writer")
	}
}
