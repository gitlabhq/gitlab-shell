package githttp

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/git"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/pktline"
)

const service = "git-receive-pack"

var receivePackHttpPrefix = []byte("001f# service=git-receive-pack\n0000")

type PushCommand struct {
	Config     *config.Config
	ReadWriter *readwriter.ReadWriter
	Response   *accessverifier.Response
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
func (c *PushCommand) Execute(ctx context.Context) error {
	data := c.Response.Payload.Data
	client, err := git.NewClient(c.Config, data.PrimaryRepo, data.RequestHeaders)
	if err != nil {
		return err
	}

	if err := c.requestInfoRefs(ctx, client); err != nil {
		return err
	}

	return c.requestReceivePack(ctx, client)
}

func (c *PushCommand) requestInfoRefs(ctx context.Context, client *git.Client) error {
	response, err := client.InfoRefs(ctx, service)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	// Read the first bytes that contain 001f# service=git-receive-pack\n0000 string
	// to convert HTTP(S) Git response to the one expected by SSH
	p := make([]byte, len(receivePackHttpPrefix))
	_, err = response.Body.Read(p)
	if err != nil || !bytes.Equal(p, receivePackHttpPrefix) {
		return fmt.Errorf("Unexpected git-receive-pack response")
	}

	_, err = io.Copy(c.ReadWriter.Out, response.Body)

	return err
}

func (c *PushCommand) requestReceivePack(ctx context.Context, client *git.Client) error {
	pipeReader, pipeWriter := io.Pipe()
	go c.readFromStdin(pipeWriter)

	response, err := client.ReceivePack(ctx, pipeReader)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	_, err = io.Copy(c.ReadWriter.Out, response.Body)

	return err
}

func (c *PushCommand) readFromStdin(pw *io.PipeWriter) {
	var needsPackData bool

	scanner := pktline.NewScanner(c.ReadWriter.In)
	for scanner.Scan() {
		line := scanner.Bytes()
		pw.Write(line)

		if pktline.IsFlush(line) {
			break
		}

		if !needsPackData && !pktline.IsRefRemoval(line) {
			needsPackData = true
		}
	}

	if needsPackData {
		io.Copy(pw, c.ReadWriter.In)
	}

	pw.Close()
}
