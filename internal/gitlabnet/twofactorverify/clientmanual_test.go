package twofactorverify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet/discover"

	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitlab-shell/client"
	"gitlab.com/gitlab-org/gitlab-shell/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
)

func initializeManual(t *testing.T) []testserver.TestRequestHandler {
	requests := []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/two_factor_manual_otp_check",
			Handler: func(w http.ResponseWriter, r *http.Request) {
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
			},
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
	manualOtpAttempt = "123456"
)

func TestVerifyOTPByKeyId(t *testing.T) {
	client := setupManual(t)

	args := &commandargs.Shell{GitlabKeyId: "0"}
	_, _, err := client.VerifyOTP(context.Background(), args, manualOtpAttempt)
	require.NoError(t, err)
}

func TestVerifyOTPByUsername(t *testing.T) {
	client := setupManual(t)

	args := &commandargs.Shell{GitlabUsername: "jane-doe"}
	_, _, err := client.VerifyOTP(context.Background(), args, manualOtpAttempt)
	require.NoError(t, err)
}

func TestErrorMessage(t *testing.T) {
	client := setupManual(t)

	args := &commandargs.Shell{GitlabKeyId: "1"}
	_, reason, _ := client.VerifyOTP(context.Background(), args, manualOtpAttempt)
	require.Equal(t, "error message", reason)
}

func TestErrorResponses(t *testing.T) {
	client := setupManual(t)

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
			_, _, err := client.VerifyOTP(context.Background(), args, manualOtpAttempt)

			require.EqualError(t, err, tc.expectedError)
		})
	}
}

func setupManual(t *testing.T) *Client {
	requests := initializeManual(t)
	url := testserver.StartSocketHttpServer(t, requests)

	client, err := NewClient(&config.Config{GitlabUrl: url})
	require.NoError(t, err)

	return client
}
