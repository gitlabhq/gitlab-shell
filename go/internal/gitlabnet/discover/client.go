package discover

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/gitlabnet"
)

type Client struct {
	config *config.Config
	client gitlabnet.GitlabClient
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

func (c *Client) GetByCommandArgs(args *commandargs.CommandArgs) (*Response, error) {
	if args.GitlabKeyId != "" {
		return c.GetByKeyId(args.GitlabKeyId)
	} else if args.GitlabUsername != "" {
		return c.GetByUsername(args.GitlabUsername)
	} else {
		// There was no 'who' information, this  matches the ruby error
		// message.
		return nil, fmt.Errorf("who='' is invalid")
	}
}

func (c *Client) GetByKeyId(keyId string) (*Response, error) {
	params := url.Values{}
	params.Add("key_id", keyId)

	return c.getResponse(params)
}

func (c *Client) GetByUsername(username string) (*Response, error) {
	params := url.Values{}
	params.Add("username", username)

	return c.getResponse(params)
}

func (c *Client) parseResponse(resp *http.Response) (*Response, error) {
	parsedResponse := &Response{}

	if err := json.NewDecoder(resp.Body).Decode(parsedResponse); err != nil {
		return nil, err
	} else {
		return parsedResponse, nil
	}
}

func (c *Client) getResponse(params url.Values) (*Response, error) {
	path := "/discover?" + params.Encode()
	response, err := c.client.Get(path)

	if err != nil {
		return nil, err
	}

	defer response.Body.Close()
	parsedResponse, err := c.parseResponse(response)
	if err != nil {
		return nil, fmt.Errorf("Parsing failed")
	}

	return parsedResponse, nil
}

func (r *Response) IsAnonymous() bool {
	return r.UserId < 1
}
