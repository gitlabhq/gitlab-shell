package healthcheck

import (
	"encoding/json"
	"net/http"
	"testing"

	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet/testserver"

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
	client, cleanup := setup(t)
	defer cleanup()

	result, err := client.Check()
	require.NoError(t, err)
	require.Equal(t, testResponse, result)
}

func setup(t *testing.T) (*Client, func()) {
	url, cleanup := testserver.StartSocketHttpServer(t, requests)

	client, err := NewClient(&config.Config{GitlabUrl: url})
	require.NoError(t, err)

	return client, cleanup
}
