package gitlabclient

import (
	"net/http"
	"time"

	"gitlab.com/gitlab-org/go/internal/config"
)

type Client struct {
	config     *config.Config
	httpClient *http.Client
}

type DiscoverResponse struct {
}

func New() (*Client, error) {
	config, err := config.New()
	if err != nil {
		return nil, err
	}

	tr = &http.Transport{
		MaxIdleConns:       10,
		IdleConnTimeout:    30 * time.Second,
		DisableCompression: true,
	}
	httpClient = &http.Client{Transport: tr}

	return &Client{config: config, httpClient: httpClient}, nil
}

func (c *Client) Discover(gitlabId string) {

}

func (c *Client) get(path string) (*Response, error) {

}
