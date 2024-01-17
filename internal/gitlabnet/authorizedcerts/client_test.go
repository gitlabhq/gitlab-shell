package authorizedcerts

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
)

var (
	requests []testserver.TestRequestHandler
)

func init() {
	requests = []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/authorized_certs",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				switch key := r.URL.Query().Get("key"); key {
				case "key":
					body := &Response{
						Namespace: "group",
						Username:  r.URL.Query().Get("user_identifier"),
					}
					json.NewEncoder(w).Encode(body)
				case "broken-message":
					w.WriteHeader(http.StatusForbidden)
					body := &client.ErrorResponse{
						Message: "Not allowed!",
					}
					json.NewEncoder(w).Encode(body)
				case "broken-json":
					w.Write([]byte("{ \"message\": \"broken json!\""))
				case "broken-empty":
					w.WriteHeader(http.StatusForbidden)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			},
		},
	}
}

func TestGetByKey(t *testing.T) {
	client := setup(t)

	result, err := client.GetByKey(context.Background(), "user-id", "key")
	require.NoError(t, err)
	require.Equal(t, &Response{Namespace: "group", Username: "user-id"}, result)
}

func TestGetByKeyErrorResponses(t *testing.T) {
	client := setup(t)

	testCases := []struct {
		desc          string
		key           string
		expectedError string
	}{
		{
			desc:          "A response with an error message",
			key:           "broken-message",
			expectedError: "Not allowed!",
		},
		{
			desc:          "A response with bad JSON",
			key:           "broken-json",
			expectedError: "Parsing failed",
		},
		{
			desc:          "A forbidden (403) response without message",
			key:           "broken-empty",
			expectedError: "Internal API error (403)",
		},
		{
			desc:          "A not found (404) response without message",
			key:           "not-found",
			expectedError: "Internal API error (404)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			resp, err := client.GetByKey(context.Background(), "user-id", tc.key)

			require.EqualError(t, err, tc.expectedError)
			require.Nil(t, resp)
		})
	}
}

func setup(t *testing.T) *Client {
	url := testserver.StartSocketHttpServer(t, requests)

	client, err := NewClient(&config.Config{GitlabUrl: url})
	require.NoError(t, err)

	return client
}
