package twofactorverify

import (
    "context"
    "errors"
    "fmt"
    "net/http"
    "log"

    "gitlab.com/gitlab-org/gitlab-shell/v14/client"
    "gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
    "gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
    "gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet"
    "gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/discover"
)

// Client is the main struct for handling two-factor authentication operations.
type Client struct {
    config *config.Config
    client *client.GitlabNetClient
}

// Response represents the response from the two-factor authentication API.
type Response struct {
    Success bool   `json:"success"`
    Message string `json:"message"`
}

// RequestBody is the request body for the two-factor authentication API.
type RequestBody struct {
    KeyID      string `json:"key_id,omitempty"`
    UserID     int64  `json:"user_id,omitempty"`
    OTPAttempt string `json:"otp_attempt,omitempty"`
}

// NewClient creates a new Client instance.
func NewClient(config *config.Config) (*Client, error) {
    client, err := gitlabnet.GetClient(config)
    if err != nil {
        return nil, fmt.Errorf("Error creating http client: %v", err)
    }

    return &Client{config: config, client: client}, nil
}

// VerifyOTP verifies a one-time password (OTP) for a user.
func (c *Client) VerifyOTP(ctx context.Context, args *commandargs.Shell, otp string) error {
    requestBody, err := c.getRequestBody(ctx, args, otp)
    if err != nil {
        return err
    }

    response, err := c.client.Post(ctx, "/two_factor_manual_otp_check", requestBody)
    if err != nil {
        return err
    }
    defer func() {
        if err := response.Body.Close(); err != nil {
            log.Printf("Error closing response body: %v", err)
        }
    }()

    return parse(response)
}

// PushAuth handles two-factor authentication via push notification.
func (c *Client) PushAuth(ctx context.Context, args *commandargs.Shell) error {
    requestBody, err := c.getRequestBody(ctx, args, "")
    if err != nil {
        return err
    }

    response, err := c.client.Post(ctx, "/two_factor_push_otp_check", requestBody)
    if err != nil {
        return err
    }
    defer func() {
        if err := response.Body.Close(); err != nil {
            log.Printf("Error closing response body: %v", err)
        }
    }()

    return parse(response)
}

func parse(hr *http.Response) error {
    response := &Response{}
    if err := gitlabnet.ParseJSON(hr, response); err != nil {
        return err
    }

    if !response.Success {
        return errors.New(response.Message)
    }

    return nil
}

func (c *Client) getRequestBody(ctx context.Context, args *commandargs.Shell, otp string) (*RequestBody, error) {
    client, err := discover.NewClient(c.config)

    if err != nil {
        return nil, err
    }

    var requestBody *RequestBody
    if args.GitlabKeyId != "" {
        requestBody = &