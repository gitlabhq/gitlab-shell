package healthcheck

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"

	"github.com/stretchr/testify/require"
)

var (
	requests = []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/check",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(testResponse)
			},
		},
	}

	testResponse = &Response{
		APIVersion:     "v4",
		GitlabVersion:  "v12.0.0-ee",
		GitlabRevision: "3b13818e8330f68625d80d9bf5d8049c41fbe197",
		Redis:          true,
	}
)

func TestCheck(t *testing.T) {
	client := setup(t)

	result, err := client.Check(context.Background())
	require.NoError(t, err)
	require.Equal(t, testResponse, result)
}

func setup(t *testing.T) *Client {
	url := testserver.StartSocketHttpServer(t, requests)

	client, err := NewClient(&config.Config{GitlabUrl: url})
	require.NoError(t, err)

	return client
}
