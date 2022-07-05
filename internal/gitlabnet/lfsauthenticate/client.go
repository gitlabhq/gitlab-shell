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

type Client struct {
	config *config.Config
	client *client.GitlabNetClient
	args   *commandargs.Shell
}

type Request struct {
	Operation string `json:"operation"`
	Repo      string `json:"project"`
	KeyId     string `json:"key_id,omitempty"`
	UserId    string `json:"user_id,omitempty"`
}

type Response struct {
	Username  string `json:"username"`
	LfsToken  string `json:"lfs_token"`
	RepoPath  string `json:"repository_http_path"`
	ExpiresIn int    `json:"expires_in"`
}

func NewClient(config *config.Config, args *commandargs.Shell) (*Client, error) {
	client, err := gitlabnet.GetClient(config)
	if err != nil {
		return nil, fmt.Errorf("Error creating http client: %v", err)
	}

	return &Client{config: config, client: client, args: args}, nil
}

func (c *Client) Authenticate(ctx context.Context, operation, repo, userId string) (*Response, error) {
	request := &Request{Operation: operation, Repo: repo}
	if c.args.GitlabKeyId != "" {
		request.KeyId = c.args.GitlabKeyId
	} else {
		request.UserId = strings.TrimPrefix(userId, "user-")
	}

	response, err := c.client.Post(ctx, "/lfs_authenticate", request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	return parse(response)
}

func parse(hr *http.Response) (*Response, error) {
	response := &Response{}
	if err := gitlabnet.ParseJSON(hr, response); err != nil {
		return nil, err
	}

	return response, nil
}
