// Package twofactorrecover provides functionality for interacting with GitLab Two-Factor Authentication recovery codes
package twofactorrecover

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/discover"
)

// Client represents a client for interacting with GitLab Two-Factor Authentication recovery codes
type Client struct {
	config *config.Config
	client *client.GitlabNetClient
}

// Response represents the response structure for Two-Factor Authentication recovery code requests
type Response struct {
	Success       bool     `json:"success"`
	RecoveryCodes []string `json:"recovery_codes"`
	Message       string   `json:"message"`
}

// RequestBody represents the request body structure for Two-Factor Authentication recovery code requests
type RequestBody struct {
	KeyID  string `json:"key_id,omitempty"`
	UserID int64  `json:"user_id,omitempty"`
}

// NewClient creates a new Client instance with the provided configuration
func NewClient(config *config.Config) (*Client, error) {
	client, err := gitlabnet.GetClient(config)
	if err != nil {
		return nil, fmt.Errorf("error creating http client: %v", err)
	}

	return &Client{config: config, client: client}, nil
}

// GetRecoveryCodes retrieves the recovery codes for the specified user
func (c *Client) GetRecoveryCodes(ctx context.Context, args *commandargs.Shell) ([]string, error) {
	requestBody, err := c.getRequestBody(ctx, args)

	if err != nil {
		return nil, err
	}

	response, err := c.client.Post(ctx, "/two_factor_recovery_codes", requestBody)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()

	return parse(response)
}

func parse(hr *http.Response) ([]string, error) {
	response := &Response{}
	if err := gitlabnet.ParseJSON(hr, response); err != nil {
		return nil, err
	}

	if !response.Success {
		return nil, errors.New(response.Message)
	}

	return response.RecoveryCodes, nil
}

func (c *Client) getRequestBody(ctx context.Context, args *commandargs.Shell) (*RequestBody, error) {
	client, err := discover.NewClient(c.config)

	if err != nil {
		return nil, err
	}

	var requestBody *RequestBody
	if args.GitlabKeyId != "" {
		requestBody = &RequestBody{KeyID: args.GitlabKeyId}
	} else {
		userInfo, err := client.GetByCommandArgs(ctx, args)

		if err != nil {
			return nil, err
		}

		requestBody = &RequestBody{UserID: userInfo.UserID}
	}

	return requestBody, nil
}
