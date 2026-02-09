package client

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
)

var (
	requests = []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/check",
			Handler: func(w http.ResponseWriter, _ *http.Request) {
				json.NewEncoder(w).Encode(testResponse)
			},
		},
	}

	testResponse = &HealthResponse{
		APIVersion:     "v4",
		GitlabVersion:  "v12.0.0-ee",
		GitlabRevision: "3b13818e8330f68625d80d9bf5d8049c41fbe197",
		Redis:          true,
	}
)

func TestCheck(t *testing.T) {
	url := testserver.StartSocketHTTPServer(t, requests)
	client, err := New(ClientOpts{
		GitlabURL: url,
	})
	require.NoError(t, err)

	result, err := client.CheckHealth(context.Background())
	require.NoError(t, err)
	require.Equal(t, testResponse, result)
}
