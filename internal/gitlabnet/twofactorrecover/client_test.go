package twofactorrecover

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/discover"
)

var (
	requests []testserver.TestRequestHandler
)

func initialize(t *testing.T) {
	requests = []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/two_factor_recovery_codes",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				b, err := io.ReadAll(r.Body)
				defer r.Body.Close()

				require.NoError(t, err)

				var requestBody *RequestBody
				json.Unmarshal(b, &requestBody)

				switch requestBody.KeyId {
				case "0":
					body := map[string]interface{}{
						"success":        true,
						"recovery_codes": [2]string{"recovery 1", "codes 1"},
					}
					json.NewEncoder(w).Encode(body)
				case "1":
					body := map[string]interface{}{
						"success": false,
						"message": "missing user",
					}
					json.NewEncoder(w).Encode(body)
				case "2":
					w.WriteHeader(http.StatusForbidden)
					body := &client.ErrorResponse{
						Message: "Not allowed!",
					}
					json.NewEncoder(w).Encode(body)
				case "3":
					w.Write([]byte("{ \"message\": \"broken json!\""))
				case "4":
					w.WriteHeader(http.StatusForbidden)
				}

				if requestBody.UserId == 1 {
					body := map[string]interface{}{
						"success":        true,
						"recovery_codes": [2]string{"recovery 2", "codes 2"},
					}
					json.NewEncoder(w).Encode(body)
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
				json.NewEncoder(w).Encode(body)
			},
		},
	}
}

func TestGetRecoveryCodesByKeyId(t *testing.T) {
	client := setup(t)

	args := &commandargs.Shell{GitlabKeyId: "0"}
	result, err := client.GetRecoveryCodes(context.Background(), args)
	require.NoError(t, err)
	require.Equal(t, []string{"recovery 1", "codes 1"}, result)
}

func TestGetRecoveryCodesByUsername(t *testing.T) {
	client := setup(t)

	args := &commandargs.Shell{GitlabUsername: "jane-doe"}
	result, err := client.GetRecoveryCodes(context.Background(), args)
	require.NoError(t, err)
	require.Equal(t, []string{"recovery 2", "codes 2"}, result)
}

func TestMissingUser(t *testing.T) {
	client := setup(t)

	args := &commandargs.Shell{GitlabKeyId: "1"}
	_, err := client.GetRecoveryCodes(context.Background(), args)
	require.Equal(t, "missing user", err.Error())
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
			resp, err := client.GetRecoveryCodes(context.Background(), args)

			require.EqualError(t, err, tc.expectedError)
			require.Nil(t, resp)
		})
	}
}

func setup(t *testing.T) *Client {
	initialize(t)
	url := testserver.StartSocketHttpServer(t, requests)

	client, err := NewClient(&config.Config{GitlabUrl: url})
	require.NoError(t, err)

	return client
}
