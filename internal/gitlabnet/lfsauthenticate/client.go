// Package lfsauthenticate provides functionality for authenticating Large File Storage (LFS) requests
package lfsauthenticate

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet"
)

// Client represents a client for LFS authentication
type Client struct {
	config *config.Config
	client *client.GitlabNetClient
	args   *commandargs.Shell
}

// Request represents a request for LFS authentication
type Request struct {
	Operation string `json:"operation"`
	Repo      string `json:"project"`
	KeyID     string `json:"key_id,omitempty"`
	UserID    string `json:"user_id,omitempty"`
}

// Response represents a response from LFS authentication
type Response struct {
	Username  string `json:"username"`
	LfsToken  string `json:"lfs_token"`
	RepoPath  string `json:"repository_http_path"`
	ExpiresIn int    `json:"expires_in"`
}

// NewClient creates a new LFS authentication client
func NewClient(config *config.Config, args *commandargs.Shell) (*Client, error) {
	client, err := gitlabnet.GetClient(config)
	if err != nil {
		return nil, fmt.Errorf("error creating http client: %v", err)
	}

	return &Client{config: config, client: client, args: args}, nil
}

// Authenticate performs authentication for LFS requests
func (c *Client) Authenticate(ctx context.Context, operation, repo, userID string) (*Response, error) {
	request := &Request{Operation: operation, Repo: repo}
	if c.args.GitlabKeyID != "" {
		request.KeyID = c.args.GitlabKeyID
	} else {
		request.UserID = strings.TrimPrefix(userID, "user-")
	}

	response, err := c.client.Post(ctx, "/lfs_authenticate", request)
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

	return response, nil
}
