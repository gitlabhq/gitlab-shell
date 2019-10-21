package healthcheck

import (
	"fmt"
	"net/http"

	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet"
)

const (
	checkPath = "/check"
)

type Client struct {
	config *config.Config
	client *gitlabnet.GitlabClient
}

type Response struct {
	APIVersion     string `json:"api_version"`
	GitlabVersion  string `json:"gitlab_version"`
	GitlabRevision string `json:"gitlab_rev"`
	Redis          bool   `json:"redis"`
}

func NewClient(config *config.Config) (*Client, error) {
	client, err := gitlabnet.GetClient(config)
	if err != nil {
		return nil, fmt.Errorf("Error creating http client: %v", err)
	}

	return &Client{config: config, client: client}, nil
}

func (c *Client) Check() (*Response, error) {
	resp, err := c.client.Get(checkPath)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	return parse(resp)
}

func parse(hr *http.Response) (*Response, error) {
	response := &Response{}
	if err := gitlabnet.ParseJSON(hr, response); err != nil {
		return nil, err
	}

	return response, nil
}
