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

type Client struct {
	config *config.Config
	client *client.GitlabNetClient
}

type Response struct {
	UserId   int64  `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
}

func NewClient(config *config.Config) (*Client, error) {
	client, err := gitlabnet.GetClient(config)
	if err != nil {
		return nil, fmt.Errorf("Error creating http client: %v", err)
	}

	return &Client{config: config, client: client}, nil
}

func (c *Client) GetByCommandArgs(ctx context.Context, args *commandargs.Shell) (*Response, error) {
	params := url.Values{}
	if args.GitlabUsername != "" {
		params.Add("username", args.GitlabUsername)
	} else if args.GitlabKeyId != "" {
		params.Add("key_id", args.GitlabKeyId)
	} else if args.GitlabKrb5Principal != "" {
		params.Add("krb5principal", args.GitlabKrb5Principal)
	} else {
		// There was no 'who' information, this  matches the ruby error
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

func (r *Response) IsAnonymous() bool {
	return r.UserId < 1
}
