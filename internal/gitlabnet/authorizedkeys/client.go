// Package authorizedkeys provides functionality for interacting with authorized keys.
package authorizedkeys

import (
	"context"
	"fmt"
	"net/url"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/clients/gitlab"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
)

const (
	// AuthorizedKeysPath represents the path to authorized keys endpoint
	AuthorizedKeysPath = "/authorized_keys"
)

// Client represents a client for interacting with authorized keys
type Client struct {
	client *gitlab.Client
}

// Response represents the response structure for authorized keys
type Response struct {
	ID  int64  `json:"id"`
	Key string `json:"key"`
}

// NewClient creates a new instance of the authorized keys client
func NewClient(config *config.Config) (*Client, error) {
	client, err := gitlab.New(&gitlab.Config{
		GitlabURL:          config.GitlabURL,
		RelativeURLRoot:    config.GitlabRelativeURLRoot,
		User:               config.HTTPSettings.User,
		Password:           config.HTTPSettings.Password,
		Secret:             config.Secret,
		CaFile:             config.HTTPSettings.CaFile,
		CaPath:             config.HTTPSettings.CaPath,
		ReadTimeoutSeconds: config.HTTPSettings.ReadTimeoutSeconds,
	})
	if err != nil {
		return nil, fmt.Errorf("error creating http client: %v", err)
	}

	return &Client{client: client}, nil
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
	if err := gitlab.ParseJSON(response, parsedResponse); err != nil {
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
