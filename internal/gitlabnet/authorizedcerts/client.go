package authorizedcerts

import (
	"context"
	"fmt"
	"net/url"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet"
)

const (
	AuthorizedCertsPath = "/authorized_certs"
)

type Client struct {
	config *config.Config
	client *client.GitlabNetClient
}

type Response struct {
	Username  string `json:"username"`
	Namespace string `json:"namespace"`
}

func NewClient(config *config.Config) (*Client, error) {
	client, err := gitlabnet.GetClient(config)
	if err != nil {
		return nil, fmt.Errorf("Error creating http client: %v", err)
	}

	return &Client{config: config, client: client}, nil
}

func (c *Client) GetByKey(ctx context.Context, userId, fingerprint string) (*Response, error) {
	path, err := pathWithKey(userId, fingerprint)
	if err != nil {
		return nil, err
	}

	response, err := c.client.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	parsedResponse := &Response{}
	if err := gitlabnet.ParseJSON(response, parsedResponse); err != nil {
		return nil, err
	}

	return parsedResponse, nil
}

func pathWithKey(userId, fingerprint string) (string, error) {
	u, err := url.Parse(AuthorizedCertsPath)
	if err != nil {
		return "", err
	}

	params := u.Query()
	params.Set("key", fingerprint)
	params.Set("user_identifier", userId)
	u.RawQuery = params.Encode()

	return u.String(), nil
}
