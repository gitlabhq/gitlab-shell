// Package authorizedkeys provides functionality for interacting with authorized keys.
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
	// AuthorizedKeysPath represents the path to authorized keys endpoint
	AuthorizedKeysPath = "/authorized_keys"
)

// Client represents a client for interacting with authorized keys
type Client struct {
	config *config.Config
	client *client.GitlabNetClient
}

// Response represents the response structure for authorized keys
type Response struct {
	ID  int64  `json:"id"`
	Key string `json:"key"`
}

// NewClient creates a new instance of the authorized keys client
func NewClient(config *config.Config) (*Client, error) {
	client, err := gitlabnet.GetClient(config)
	if err != nil {
		return nil, fmt.Errorf("error creating http client: %v", err)
	}

	return &Client{config: config, client: client}, nil
}

// GetByKey retrieves authorized keys by key
func (c *Client) GetByKey(ctx context.Context, key string) (*Response, error) {
	path, err := pathWithKey(key)
	if err != nil {
		return nil, err
	}

	response, err := c.client.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()

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
