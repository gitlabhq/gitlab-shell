package twofactorverify

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"gitlab.com/gitlab-org/gitlab-shell/client"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet/discover"
)

type Client struct {
	config *config.Config
	client *client.GitlabNetClient
}

type Response struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type RequestBody struct {
	KeyId      string `json:"key_id,omitempty"`
	UserId     int64  `json:"user_id,omitempty"`
	OTPAttempt string `json:"otp_attempt"`
	PushAuth   string `json:"push_auth"`
}

func NewClient(config *config.Config) (*Client, error) {
	client, err := gitlabnet.GetClient(config)
	if err != nil {
		return nil, fmt.Errorf("Error creating http client: %v", err)
	}

	return &Client{config: config, client: client}, nil
}

func (c *Client) VerifyOTP(ctx context.Context, args *commandargs.Shell, otp string) (bool, string, error) {
	requestBody, err := c.getRequestBody(ctx, args, otp, false)
	if err != nil {
		return false, "", err
	}

	response, err := c.client.Post(ctx, "/two_factor_otp_check", requestBody)
	if err != nil {
		return false, "", err
	}
	defer response.Body.Close()

	return parse(response)
}

func (c *Client) PushAuth(ctx context.Context, args *commandargs.Shell) (bool, string, error) {
	// enable push auth in internal rest api
	requestBody, err := c.getRequestBody(ctx, args, "", true)
	if err != nil {
		return false, "", err
	}

	response, err := c.client.Post(ctx, "/two_factor_otp_check", requestBody)
	if err != nil {
		return false, "", err
	}
	defer response.Body.Close()

	return parse(response)
}

func parse(hr *http.Response) (bool, string, error) {
	response := &Response{}
	if err := gitlabnet.ParseJSON(hr, response); err != nil {
		return false, "", err
	}

	if !response.Success {
		return false, response.Message, nil
	}

	return true, response.Message, nil
}

func (c *Client) getRequestBody(ctx context.Context, args *commandargs.Shell, otp string, pushauth bool) (*RequestBody, error) {
	client, err := discover.NewClient(c.config)

	if err != nil {
		return nil, err
	}

	var requestBody *RequestBody
	if args.GitlabKeyId != "" {
		requestBody = &RequestBody{KeyId: args.GitlabKeyId, OTPAttempt: otp, PushAuth: strconv.FormatBool(pushauth)}
	} else {
		userInfo, err := client.GetByCommandArgs(ctx, args)

		if err != nil {
			return nil, err
		}

		requestBody = &RequestBody{UserId: userInfo.UserId, OTPAttempt: otp, PushAuth: strconv.FormatBool(pushauth)}
	}

	return requestBody, nil
}
