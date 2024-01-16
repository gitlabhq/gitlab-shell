// Package authorizedcerts implements functions for authorizing users with ssh certificates
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
	authorizedCertsPath = "/authorized_certs"
)

// Client wraps a gitlab client and its associated config
type Client struct {
	config *config.Config
	client *client.GitlabNetClient
}

// Response contains the json response from authorized_certs
type Response struct {
	Username  string `json:"username"`
	Namespace string `json:"namespace"`
}

// NewClient instantiates a Client with config
func NewClient(config *config.Config) (*Client, error) {
	client, err := gitlabnet.GetClient(config)
	if err != nil {
		return nil, fmt.Errorf("error creating http client: %v", err)
	}

	return &Client{config: config, client: client}, nil
}

// GetByKey makes a request to authorized_certs for the namespace configured with a cert that matches fingerprint
func (c *Client) GetByKey(ctx context.Context, userID, fingerprint string) (*Response, error) {
	path, err := pathWithKey(userID, fingerprint)
	if err != nil {
		return nil, err
	}

	response, err := c.client.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = response.Body.Close()
	}()

	parsedResponse := &Response{}
	if err := gitlabnet.ParseJSON(response, parsedResponse); err != nil {
		return nil, err
	}

	return parsedResponse, nil
}

func pathWithKey(userID, fingerprint string) (string, error) {
	u, err := url.Parse(authorizedCertsPath)
	if err != nil {
		return "", err
	}

	params := u.Query()
	params.Set("key", fingerprint)
	params.Set("user_identifier", userID)
	u.RawQuery = params.Encode()

	return u.String(), nil
}
