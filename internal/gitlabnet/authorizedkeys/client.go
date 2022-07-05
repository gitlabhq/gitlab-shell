package authorizedkeys

import (
	"context"
	"fmt"
	"net/url"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet"
)

const (
	AuthorizedKeysPath = "/authorized_keys"
)

type Client struct {
	config *config.Config
	client *client.GitlabNetClient
}

type Response struct {
	Id  int64  `json:"id"`
	Key string `json:"key"`
}

func NewClient(config *config.Config) (*Client, error) {
	client, err := gitlabnet.GetClient(config)
	if err != nil {
		return nil, fmt.Errorf("Error creating http client: %v", err)
	}

	return &Client{config: config, client: client}, nil
}

func (c *Client) GetByKey(ctx context.Context, key string) (*Response, error) {
	path, err := pathWithKey(key)
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

func pathWithKey(key string) (string, error) {
	u, err := url.Parse(AuthorizedKeysPath)
	if err != nil {
		return "", err
	}

	params := u.Query()
	params.Set("key", key)
	u.RawQuery = params.Encode()

	return u.String(), nil
}
