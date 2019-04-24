package discover

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/gitlabnet"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/gitlabnet/testserver"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	requests []testserver.TestRequestHandler
)

func init() {
	requests = []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/discover",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("key_id") == "1" {
					body := &Response{
						UserId:   2,
						Username: "alex-doe",
						Name:     "Alex Doe",
					}
					json.NewEncoder(w).Encode(body)
				} else if r.URL.Query().Get("username") == "jane-doe" {
					body := &Response{
						UserId:   1,
						Username: "jane-doe",
						Name:     "Jane Doe",
					}
					json.NewEncoder(w).Encode(body)
				} else if r.URL.Query().Get("username") == "broken_message" {
					w.WriteHeader(http.StatusForbidden)
					body := &gitlabnet.ErrorResponse{
						Message: "Not allowed!",
					}
					json.NewEncoder(w).Encode(body)
				} else if r.URL.Query().Get("username") == "broken_json" {
					w.Write([]byte("{ \"message\": \"broken json!\""))
				} else if r.URL.Query().Get("username") == "broken_empty" {
					w.WriteHeader(http.StatusForbidden)
				} else {
					fmt.Fprint(w, "null")
				}
			},
		},
	}
}

func TestGetByKeyId(t *testing.T) {
	client, cleanup := setup(t)
	defer cleanup()

	result, err := client.GetByKeyId("1")
	assert.NoError(t, err)
	assert.Equal(t, &Response{UserId: 2, Username: "alex-doe", Name: "Alex Doe"}, result)
}

func TestGetByUsername(t *testing.T) {
	client, cleanup := setup(t)
	defer cleanup()

	result, err := client.GetByUsername("jane-doe")
	assert.NoError(t, err)
	assert.Equal(t, &Response{UserId: 1, Username: "jane-doe", Name: "Jane Doe"}, result)
}

func TestMissingUser(t *testing.T) {
	client, cleanup := setup(t)
	defer cleanup()

	result, err := client.GetByUsername("missing")
	assert.NoError(t, err)
	assert.True(t, result.IsAnonymous())
}

func TestErrorResponses(t *testing.T) {
	client, cleanup := setup(t)
	defer cleanup()

	testCases := []struct {
		desc          string
		fakeUsername  string
		expectedError string
	}{
		{
			desc:          "A response with an error message",
			fakeUsername:  "broken_message",
			expectedError: "Not allowed!",
		},
		{
			desc:          "A response with bad JSON",
			fakeUsername:  "broken_json",
			expectedError: "Parsing failed",
		},
		{
			desc:          "An error response without message",
			fakeUsername:  "broken_empty",
			expectedError: "Internal API error (403)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			resp, err := client.GetByUsername(tc.fakeUsername)

			assert.EqualError(t, err, tc.expectedError)
			assert.Nil(t, resp)
		})
	}
}

func setup(t *testing.T) (*Client, func()) {
	cleanup, url, err := testserver.StartSocketHttpServer(requests)
	require.NoError(t, err)

	client, err := NewClient(&config.Config{GitlabUrl: url})
	require.NoError(t, err)

	return client, cleanup
}
