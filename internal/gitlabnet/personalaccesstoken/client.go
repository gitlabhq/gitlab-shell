// Package personalaccesstoken provides functionality for managing personal access tokens
package personalaccesstoken

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/discover"
)

// Client represents a client for managing personal access tokens
type Client struct {
	config *config.Config
	client *client.GitlabNetClient
}

// Response represents the response from creating a personal access token
type Response struct {
	Success   bool     `json:"success"`
	Token     string   `json:"token"`
	Scopes    []string `json:"scopes"`
	ExpiresAt string   `json:"expires_at"`
	Message   string   `json:"message"`
}

// RequestBody represents the request body for creating a personal access token
type RequestBody struct {
	KeyID     string   `json:"key_id,omitempty"`
	UserID    int64    `json:"user_id,omitempty"`
	Name      string   `json:"name"`
	Scopes    []string `json:"scopes"`
	ExpiresAt string   `json:"expires_at,omitempty"`
}

// NewClient creates a new instance of Client
func NewClient(config *config.Config) (*Client, error) {
	client, err := gitlabnet.GetClient(config)
	if err != nil {
		return nil, fmt.Errorf("error creating http client: %v", err)
	}

	return &Client{config: config, client: client}, nil
}

// GetPersonalAccessToken retrieves or creates a personal access token
func (c *Client) GetPersonalAccessToken(ctx context.Context, args *commandargs.Shell, name string, scopes *[]string, expiresAt string) (*Response, error) {
	requestBody, err := c.getRequestBody(ctx, args, name, scopes, expiresAt)
	if err != nil {
		return nil, err
	}

	response, err := c.client.Post(ctx, "/personal_access_token", requestBody)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()

	return parse(response)
}

func parse(hr *http.Response) (*Response, error) {
	response := &Response{}
	if err := gitlabnet.ParseJSON(hr, response); err != nil {
		return nil, err
	}

	if !response.Success {
		return nil, errors.New(response.Message)
	}

	return response, nil
}

func (c *Client) getRequestBody(ctx context.Context, args *commandargs.Shell, name string, scopes *[]string, expiresAt string) (*RequestBody, error) {
	client, err := discover.NewClient(c.config)
	if err != nil {
		return nil, err
	}

	requestBody := &RequestBody{Name: name, Scopes: *scopes, ExpiresAt: expiresAt}
	if args.GitlabKeyId != "" {
		requestBody.KeyID = args.GitlabKeyId

		return requestBody, nil
	}

	userInfo, err := client.GetByCommandArgs(ctx, args)
	if err != nil {
		return nil, err
	}
	requestBody.UserID = userInfo.UserID

	return requestBody, nil
}
