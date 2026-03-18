// Package authorizedcerts implements functions for authorizing users with ssh certificates
package authorizedcerts

import (
	"context"
	"fmt"
	"net/url"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/clients/gitlab"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
)

const (
	authorizedCertsPath = "/authorized_certs"
)

// Client wraps a gitlab client and its associated config
type Client struct {
	client *gitlab.Client
}

// Response contains the json response from authorized_certs
type Response struct {
	Username  string `json:"username"`
	Namespace string `json:"namespace"`
}

// NewClient instantiates a Client with config
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
	if err := gitlab.ParseJSON(response, parsedResponse); err != nil {
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
