package twofactorverify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/discover"

	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
)

func initialize(t *testing.T) []testserver.TestRequestHandler {
	handler := func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		defer r.Body.Close()

		require.NoError(t, err)

		var requestBody *RequestBody
		require.NoError(t, json.Unmarshal(b, &requestBody))

		switch requestBody.KeyId {
		case "0":
			body := map[string]interface{}{
				"success": true,
			}
			require.NoError(t, json.NewEncoder(w).Encode(body))
		case "1":
			body := map[string]interface{}{
				"success": false,
				"message": "error message",
			}
			require.NoError(t, json.NewEncoder(w).Encode(body))
		case "2":
			w.WriteHeader(http.StatusForbidden)
			body := &client.ErrorResponse{
				Message: "Not allowed!",
			}
			require.NoError(t, json.NewEncoder(w).Encode(body))
		case "3":
			w.Write([]byte("{ \"message\": \"broken json!\""))
		case "4":
			w.WriteHeader(http.StatusForbidden)
		}

		if requestBody.UserId == 1 {
			body := map[string]interface{}{
				"success": true,
			}
			require.NoError(t, json.NewEncoder(w).Encode(body))
		}
	}

	requests := []testserver.TestRequestHandler{
		{
			Path:    "/api/v4/internal/two_factor_manual_otp_check",
			Handler: handler,
		},
		{
			Path:    "/api/v4/internal/two_factor_push_otp_check",
			Handler: handler,
		},
		{
			Path: "/api/v4/internal/discover",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				body := &discover.Response{
					UserId:   1,
					Username: "jane-doe",
					Name:     "Jane Doe",
				}
				require.NoError(t, json.NewEncoder(w).Encode(body))
			},
		},
	}

	return requests
}

const (
	otpAttempt = "123456"
)

func TestVerifyOTPByKeyId(t *testing.T) {
	client := setup(t)

	args := &commandargs.Shell{GitlabKeyId: "0"}
	err := client.VerifyOTP(context.Background(), args, otpAttempt)
	require.NoError(t, err)
}

func TestVerifyOTPByUsername(t *testing.T) {
	client := setup(t)

	args := &commandargs.Shell{GitlabUsername: "jane-doe"}
	err := client.VerifyOTP(context.Background(), args, otpAttempt)
	require.NoError(t, err)
}

func TestErrorMessage(t *testing.T) {
	client := setup(t)

	args := &commandargs.Shell{GitlabKeyId: "1"}
	err := client.VerifyOTP(context.Background(), args, otpAttempt)
	require.Equal(t, "error message", err.Error())
}

func TestErrorResponses(t *testing.T) {
	client := setup(t)

	testCases := []struct {
		desc          string
		fakeId        string
		expectedError string
	}{
		{
			desc:          "A response with an error message",
			fakeId:        "2",
			expectedError: "Not allowed!",
		},
		{
			desc:          "A response with bad JSON",
			fakeId:        "3",
			expectedError: "Parsing failed",
		},
		{
			desc:          "An error response without message",
			fakeId:        "4",
			expectedError: "Internal API error (403)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			args := &commandargs.Shell{GitlabKeyId: tc.fakeId}
			err := client.VerifyOTP(context.Background(), args, otpAttempt)

			require.EqualError(t, err, tc.expectedError)
		})
	}
}

func TestVerifyPush(t *testing.T) {
	client := setup(t)

	args := &commandargs.Shell{GitlabKeyId: "0"}
	err := client.PushAuth(context.Background(), args)
	require.NoError(t, err)
}

func TestErrorMessagePush(t *testing.T) {
	client := setup(t)

	args := &commandargs.Shell{GitlabKeyId: "1"}
	err := client.PushAuth(context.Background(), args)
	require.Equal(t, "error message", err.Error())
}

func TestErrorResponsesPush(t *testing.T) {
	client := setup(t)

	testCases := []struct {
		desc          string
		fakeId        string
		expectedError string
	}{
		{
			desc:          "A response with an error message",
			fakeId:        "2",
			expectedError: "Not allowed!",
		},
		{
			desc:          "A response with bad JSON",
			fakeId:        "3",
			expectedError: "Parsing failed",
		},
		{
			desc:          "An error response without message",
			fakeId:        "4",
			expectedError: "Internal API error (403)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			args := &commandargs.Shell{GitlabKeyId: tc.fakeId}
			err := client.PushAuth(context.Background(), args)

			require.EqualError(t, err, tc.expectedError)
		})
	}
}

func setup(t *testing.T) *Client {
	requests := initialize(t)
	url := testserver.StartSocketHttpServer(t, requests)

	client, err := NewClient(&config.Config{GitlabUrl: url})
	require.NoError(t, err)

	return client
}
