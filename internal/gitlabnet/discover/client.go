// Package discover provides functionality for discovering GitLab users
package discover

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet"
)

// Client represents a client for discovering GitLab users
type Client struct {
	config *config.Config
	client *client.GitlabNetClient
}

// Response represents the response structure for user discovery
type Response struct {
	UserID   int64  `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
}

// NewClient creates a new instance of the user discovery client
func NewClient(config *config.Config) (*Client, error) {
	client, err := gitlabnet.GetClient(config)
	if err != nil {
		return nil, fmt.Errorf("error creating http client: %v", err)
	}

	return &Client{config: config, client: client}, nil
}

// GetByCommandArgs retrieves user information based on command arguments
func (c *Client) GetByCommandArgs(ctx context.Context, args *commandargs.Shell) (*Response, error) {
	params := url.Values{}
	switch {
	case args.GitlabUsername != "":
		params.Add("username", args.GitlabUsername)
	case args.GitlabKeyID != "":
		params.Add("key_id", args.GitlabKeyID)
	case args.GitlabKrb5Principal != "":
		params.Add("krb5principal", args.GitlabKrb5Principal)
	default:
		// There was no 'who' information, this matches the ruby error
		// message.
		return nil, fmt.Errorf("who='' is invalid")
	}

	return c.getResponse(ctx, params)
}

func (c *Client) getResponse(ctx context.Context, params url.Values) (*Response, error) {
	path := "/discover?" + params.Encode()

	response, err := c.client.Get(ctx, path)
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

// IsAnonymous checks if the user is anonymous
func (r *Response) IsAnonymous() bool {
	return r.UserID < 1
}
