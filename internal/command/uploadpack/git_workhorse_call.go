package uploadpack

import (
	"context"
	"io"
	"net/http"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/accessverifier"
)

func (c *Command) performWorkhorseCall(ctx context.Context, response *accessverifier.Response) error {
	client := &http.Client{}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, response.GitRpcUrl, io.NopCloser(c.ReadWriter.In))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", response.GitRpcAuthHeader)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	_, err = io.Copy(c.ReadWriter.Out, resp.Body)

	return err
}
