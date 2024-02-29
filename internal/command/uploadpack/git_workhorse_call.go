package uploadpack

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"

	"golang.org/x/net/http2"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/accessverifier"
)

func (c *Command) performWorkhorseCall(ctx context.Context, response *accessverifier.Response) error {
	client := &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
				dialer := &net.Dialer{}
				return dialer.DialContext(ctx, network, addr)
			},
		},
	}

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
