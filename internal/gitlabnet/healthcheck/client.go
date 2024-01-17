// Package healthcheck implements a HTTP client to request healthcheck endpoint
package healthcheck

import (
	"context"
	"fmt"
	"net/http"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet"
)

const (
	checkPath = "/check"
)

// Client defines configuration for healthcheck client
type Client struct {
	config *config.Config
	client *client.GitlabNetClient
}

// Response contains healthcheck endpoint response data
type Response struct {
	APIVersion     string `json:"api_version"`
	GitlabVersion  string `json:"gitlab_version"`
	GitlabRevision string `json:"gitlab_rev"`
	Redis          bool   `json:"redis"`
}

// NewClient initializes a client's struct
func NewClient(config *config.Config) (*Client, error) {
	client, err := gitlabnet.GetClient(config)
	if err != nil {
		return nil, fmt.Errorf("error creating http client: %v", err)
	}

	return &Client{config: config, client: client}, nil
}

// Check makes a GET request to healthcheck endpoint
func (c *Client) Check(ctx context.Context) (response *Response, err error) {
	resp, err := c.client.Get(ctx, checkPath)
	if err != nil {
		return nil, err
	}

	defer func() {
		err = resp.Body.Close()
	}()

	return parse(resp)
}

func parse(hr *http.Response) (*Response, error) {
	response := &Response{}
	if err := gitlabnet.ParseJSON(hr, response); err != nil {
		return nil, err
	}

	return response, nil
}
