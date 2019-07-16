package authorizedkeys

import (
	"fmt"
	"net/url"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/gitlabnet"
)

type Client struct {
	config *config.Config
	client *gitlabnet.GitlabClient
}

type Response struct {
	Id  int64  `json:"id"`
	Key string `json:"key"`
}

func NewClient(config *config.Config) (*Client, error) {
	client, err := gitlabnet.GetClient(config)
	if err != nil {
		return nil, fmt.Errorf("Error creating http client: %v", err)
	}

	return &Client{config: config, client: client}, nil
}

func (c *Client) GetByKey(key string) (*Response, error) {
	params := url.Values{}
	params.Add("key", key)

	path := "/authorized_keys?" + params.Encode()
	response, err := c.client.Get(path)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	parsedResponse := &Response{}
	if err := gitlabnet.ParseJSON(response, parsedResponse); err != nil {
		return nil, err
	}

	return parsedResponse, nil
}
