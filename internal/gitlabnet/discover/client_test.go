package discover

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"

	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
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
				} else if r.URL.Query().Get("krb5principal") == "john-doe@TEST.TEST" {
					body := &Response{
						UserId:   3,
						Username: "john-doe",
						Name:     "John Doe",
					}
					json.NewEncoder(w).Encode(body)
				} else if r.URL.Query().Get("username") == "broken_message" {
					w.WriteHeader(http.StatusForbidden)
					body := &client.ErrorResponse{
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
	client := setup(t)

	params := url.Values{}
	params.Add("key_id", "1")
	result, err := client.getResponse(context.Background(), params)
	require.NoError(t, err)
	require.Equal(t, &Response{UserId: 2, Username: "alex-doe", Name: "Alex Doe"}, result)
}

func TestGetByUsername(t *testing.T) {
	client := setup(t)

	params := url.Values{}
	params.Add("username", "jane-doe")
	result, err := client.getResponse(context.Background(), params)
	require.NoError(t, err)
	require.Equal(t, &Response{UserId: 1, Username: "jane-doe", Name: "Jane Doe"}, result)
}

func TestGetByKrb5Principal(t *testing.T) {
	client := setup(t)

	params := url.Values{}
	params.Add("krb5principal", "john-doe@TEST.TEST")
	result, err := client.getResponse(context.Background(), params)
	require.NoError(t, err)
	require.Equal(t, &Response{UserId: 3, Username: "john-doe", Name: "John Doe"}, result)
}

func TestMissingUser(t *testing.T) {
	client := setup(t)

	params := url.Values{}
	params.Add("username", "missing")
	result, err := client.getResponse(context.Background(), params)
	require.NoError(t, err)
	require.True(t, result.IsAnonymous())
}

func TestErrorResponses(t *testing.T) {
	client := setup(t)

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
			params := url.Values{}
			params.Add("username", tc.fakeUsername)
			resp, err := client.getResponse(context.Background(), params)

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
