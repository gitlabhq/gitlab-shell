package uploadpack

import (
	"context"
	"io"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/git"
)

func (c *Command) performWorkhorseCall(ctx context.Context, response *accessverifier.Response) error {
	client := &git.Client{
		URL: response.GitRpcUrl,
		Headers: map[string]string{
			"Gitlab-Shell-Api-Request": response.GitRpcAuthHeader,
			"Git-Protocol":             c.Args.Env.GitProtocolVersion,
		},
	}

	resp, err := client.SshUploadPack(ctx, io.NopCloser(c.ReadWriter.In))

	if err != nil {
		return err
	}
	defer resp.Body.Close()

	_, err = io.Copy(c.ReadWriter.Out, resp.Body)

	return err
}
