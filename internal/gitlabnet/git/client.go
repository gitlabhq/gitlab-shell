package git

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet"
)

type Client struct {
	url     string
	headers map[string]string
	client  *client.GitlabNetClient
}

func NewClient(cfg *config.Config, url string, headers map[string]string) (*Client, error) {
	client, err := gitlabnet.GetClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("Error creating http client: %v", err)
	}

	return &Client{client: client, headers: headers, url: url}, nil
}

func (c *Client) InfoRefs(ctx context.Context, service string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url+"/info/refs?service="+service, nil)
	if err != nil {
		return nil, err
	}

	return c.do(request)
}

func (c *Client) ReceivePack(ctx context.Context, body io.Reader) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+"/git-receive-pack", body)
	if err != nil {
		return nil, err
	}
	request.Header.Add("Content-Type", "application/x-git-receive-pack-request")
	request.Header.Add("Accept", "application/x-git-receive-pack-result")

	return c.do(request)
}

func (c *Client) do(request *http.Request) (*http.Response, error) {

	for k, v := range c.headers {
		request.Header.Add(k, v)
	}

	return c.client.Do(request)
}
