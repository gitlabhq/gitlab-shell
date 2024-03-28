package uploadpack

import (
	"context"
	"io"
	"net/http"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/accessverifier"
)

var httpClient = &http.Client{
	Transport: client.NewTransport(client.DefaultTransport()),
}

func (c *Command) performWorkhorseCall(ctx context.Context, response *accessverifier.Response) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, response.GitRpcUrl, io.NopCloser(c.ReadWriter.In))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", response.GitRpcAuthHeader)
	req.Header.Set("Git-Protocol", c.Args.Env.GitProtocolVersion)

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	_, err = io.Copy(c.ReadWriter.Out, resp.Body)

	return err
}
