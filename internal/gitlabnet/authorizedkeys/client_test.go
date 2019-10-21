package authorizedkeys

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet/testserver"
)

var (
	requests []testserver.TestRequestHandler
)

func init() {
	requests = []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/authorized_keys",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("key") == "key" {
					body := &Response{
						Id:  1,
						Key: "public-key",
					}
					json.NewEncoder(w).Encode(body)
				} else if r.URL.Query().Get("key") == "broken-message" {
					w.WriteHeader(http.StatusForbidden)
					body := &gitlabnet.ErrorResponse{
						Message: "Not allowed!",
					}
					json.NewEncoder(w).Encode(body)
				} else if r.URL.Query().Get("key") == "broken-json" {
					w.Write([]byte("{ \"message\": \"broken json!\""))
				} else if r.URL.Query().Get("key") == "broken-empty" {
					w.WriteHeader(http.StatusForbidden)
				} else {
					w.WriteHeader(http.StatusNotFound)
				}
			},
		},
	}
}

func TestGetByKey(t *testing.T) {
	client, cleanup := setup(t)
	defer cleanup()

	result, err := client.GetByKey("key")
	require.NoError(t, err)
	require.Equal(t, &Response{Id: 1, Key: "public-key"}, result)
}

func TestGetByKeyErrorResponses(t *testing.T) {
	client, cleanup := setup(t)
	defer cleanup()

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
			resp, err := client.GetByKey(tc.key)

			require.EqualError(t, err, tc.expectedError)
			require.Nil(t, resp)
		})
	}
}

func setup(t *testing.T) (*Client, func()) {
	url, cleanup := testserver.StartSocketHttpServer(t, requests)

	client, err := NewClient(&config.Config{GitlabUrl: url})
	require.NoError(t, err)

	return client, cleanup
}
