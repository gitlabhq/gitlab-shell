package githttp

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/git"
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
