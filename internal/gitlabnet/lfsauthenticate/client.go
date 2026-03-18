// Package lfsauthenticate provides functionality for authenticating Large File Storage (LFS) requests
package lfsauthenticate

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/clients/gitlab"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
)

// Client represents a client for LFS authentication
type Client struct {
	client *gitlab.Client
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

	return &Client{client: client, args: args}, nil
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
	if err := gitlab.ParseJSON(hr, response); err != nil {
		return nil, err
	}

	return response, nil
}
