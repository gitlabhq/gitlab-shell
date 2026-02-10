package gitlab

import (
	"context"
	"errors"
	"net/http"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet"
)

type OTPVerifiedResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// OTPRequestBody represents the request body for two-factor verification.
type OTPRequestBody struct {
	KeyID      string `json:"key_id,omitempty"`
	UserID     int64  `json:"user_id,omitempty"`
	OTPAttempt string `json:"otp_attempt,omitempty"`
}

type VerifyOTPArgs struct {
	GitlabKeyID         string
	GitlabUsername      string
	GitlabKrb5Principal string

	OTPAttempt string
}

func (c *Client) VerityOTP(ctx context.Context, args VerifyOTPArgs) (*OTPVerifiedResponse, error) {
	req, err := c.toOTPRequestBody(ctx, args)
	if err != nil {
		return nil, err
	}

	resp, err := c.Do(ctx, http.MethodPost, "/two_factor_manual_otp_check", req)
	if err != nil {
		return nil, err
	}

	verifiedResponse := &OTPVerifiedResponse{}
	if err := gitlabnet.ParseJSON(resp, verifiedResponse); err != nil {
		return nil, err
	}

	if !verifiedResponse.Success {
		return nil, errors.New(verifiedResponse.Message)
	}

	return verifiedResponse, err
}

func (c *Client) PushAuth(ctx context.Context) {}

func (c *Client) GetRecoveryCodes(ctx context.Context) {}

// a helper method that constructs the OTP request body for verification, push and recovery code
// requests.
func (c *Client) toOTPRequestBody(ctx context.Context, args VerifyOTPArgs) (*OTPRequestBody, error) {
	var req *OTPRequestBody
	if args.GitlabKeyID != "" {
		req = &OTPRequestBody{
			KeyID:      args.GitlabKeyID,
			OTPAttempt: args.OTPAttempt,
		}
	} else {
		userInfo, err := c.GetUser(ctx, GetUserArgs{
			GitlabUsername:      args.GitlabUsername,
			GitlabKrb5Principal: args.GitlabKrb5Principal,
		})
		if err != nil {
			return nil, err
		}

		req = &OTPRequestBody{
			UserID:     userInfo.UserID,
			OTPAttempt: args.OTPAttempt,
		}
	}

	return req, nil
}
