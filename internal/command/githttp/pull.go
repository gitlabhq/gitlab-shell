package githttp

import (
	"bytes"
	"context"
	"fmt"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/git"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/pktline"
	"io"
)

const pullService = "git-upload-pack"

var uploadPackHttpPrefix = []byte("001e# service=git-upload-pack\n0000")

type PullCommand struct {
	Config     *config.Config
	ReadWriter *readwriter.ReadWriter
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

func (c *PullCommand) Execute(ctx context.Context) error {
	data := c.Response.Payload.Data
	client := &git.Client{Url: data.PrimaryRepo, Headers: data.RequestHeaders}

	if err := c.requestInfoRefs(ctx, client); err != nil {
		return err
	}

	return c.requestUploadPack(ctx, client)
}

func (c *PullCommand) requestInfoRefs(ctx context.Context, client *git.Client) error {
	response, err := client.InfoRefs(ctx, pullService)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	// Read the first bytes that contain 001e# service=git-upload-pack\n0000 string
	// to convert HTTP(S) Git response to the one expected by SSH
	p := make([]byte, len(uploadPackHttpPrefix))
	_, err = response.Body.Read(p)
	if err != nil || !bytes.Equal(p, uploadPackHttpPrefix) {
		return fmt.Errorf("Unexpected git-upload-pack response")
	}

	_, err = io.Copy(c.ReadWriter.Out, response.Body)

	return err
}

func (c *PullCommand) requestUploadPack(ctx context.Context, client *git.Client) error {
	pipeReader, pipeWriter := io.Pipe()
	go c.readFromStdin(pipeWriter)

	response, err := client.UploadPack(ctx, pipeReader)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	_, err = io.Copy(c.ReadWriter.Out, response.Body)

	return err
}

func (c *PullCommand) readFromStdin(pw *io.PipeWriter) {
	scanner := pktline.NewScanner(c.ReadWriter.In)

	for scanner.Scan() {
		line := scanner.Bytes()

		if pktline.IsDone(line) {
			pw.Write(line)

			break
		}

		pw.Write(line)
	}

	pw.Close()
}
