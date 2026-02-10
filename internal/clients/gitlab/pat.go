package gitlab

import (
	"context"
	"errors"
	"net/http"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet"
)

const (
	PATPath = "/personal_access_token"
)

// PersonalAccessTokenResponse represents the response from creating a personal access token
type PersonalAccessTokenResponse struct {
	Success   bool     `json:"success"`
	Token     string   `json:"token"`
	Scopes    []string `json:"scopes"`
	ExpiresAt string   `json:"expires_at"`
	Message   string   `json:"message"`
}

// PersonalAccessTokenRequest represents the request body for creating a personal access token
type PersonalAccessTokenRequest struct {
	KeyID     string   `json:"key_id,omitempty"`
	UserID    int64    `json:"user_id,omitempty"`
	Name      string   `json:"name"`
	Scopes    []string `json:"scopes"`
	ExpiresAt string   `json:"expires_at,omitempty"`
}

type GetPATArgs struct {
	GitlabKeyID         string
	GitlabUsername      string
	GitlabKrb5Principal string

	Name      string
	Scopes    []string
	ExpiresAt string
}

func (c *Client) GetPersonalAccessToken(
	ctx context.Context,
	args GetPATArgs,
) (*PersonalAccessTokenResponse, error) {
	requestBody, err := c.toPATRequest(ctx, args)
	if err != nil {
		return nil, err
	}

	resp, err := c.Do(ctx, http.MethodPost, PATPath, requestBody)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	patResponse := &PersonalAccessTokenResponse{}
	if err := gitlabnet.ParseJSON(resp, patResponse); err != nil {
		return nil, err
	}

	if !patResponse.Success {
		return nil, errors.New(patResponse.Message)
	}

	return patResponse, nil
}

func (c *Client) toPATRequest(
	ctx context.Context,
	args GetPATArgs,
) (*PersonalAccessTokenRequest, error) {
	requestBody := &PersonalAccessTokenRequest{Name: args.Name, Scopes: args.Scopes, ExpiresAt: args.ExpiresAt}
	if args.GitlabKeyID != "" {
		requestBody.KeyID = args.GitlabKeyID

		return requestBody, nil
	}

	userInfo, err := c.GetUser(ctx, GetUserArgs{
		GitlabKeyID:         args.GitlabKeyID,
		GitlabUsername:      args.GitlabUsername,
		GitlabKrb5Principal: args.GitlabKrb5Principal,
	})
	if err != nil {
		return nil, err
	}
	requestBody.UserID = userInfo.UserID

	return requestBody, nil
}
