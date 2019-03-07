package discover

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

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
	defer resp.Body.Close()
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

	return c.parseResponse(response)
}

func (r *Response) IsAnonymous() bool {
	return r.UserId < 1
}
