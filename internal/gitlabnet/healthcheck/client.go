// Package healthcheck implements a HTTP client to request healthcheck endpoint
package healthcheck

import (
	"context"
	"fmt"
	"net/http"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/clients/gitlab"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
)

const (
	checkPath = "/check"
)

// Client defines configuration for healthcheck client
type Client struct {
	client *gitlab.Client
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
	client, err := gitlab.New(gitlab.ConfigFromShellConfig(config))
	if err != nil {
		return nil, fmt.Errorf("error creating http client: %v", err)
	}

	return &Client{client: client}, nil
}

// Check makes a GET request to healthcheck endpoint
func (c *Client) Check(ctx context.Context) (*Response, error) {
	resp, err := c.client.Get(ctx, checkPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	return parse(resp)
}

func parse(hr *http.Response) (*Response, error) {
	response := &Response{}
	if err := gitlab.ParseJSON(hr, response); err != nil {
		return nil, err
	}

	return response, nil
}
