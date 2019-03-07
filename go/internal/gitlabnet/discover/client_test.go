package discover

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/gitlabnet/testserver"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testConfig *config.Config
	requests   []testserver.TestRequestHandler
)

func init() {
	testConfig = &config.Config{GitlabUrl: "http+unix://" + testserver.TestSocket}
	requests = []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/discover",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("key_id") == "1" {
					body := map[string]interface{}{
						"id":       2,
						"username": "alex-doe",
						"name":     "Alex Doe",
					}
					json.NewEncoder(w).Encode(body)
				} else if r.URL.Query().Get("username") == "jane-doe" {
					body := map[string]interface{}{
						"id":       1,
						"username": "jane-doe",
						"name":     "Jane Doe",
					}
					json.NewEncoder(w).Encode(body)
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

func setup(t *testing.T) (*Client, func()) {
	cleanup, err := testserver.StartSocketHttpServer(requests)
	require.NoError(t, err)

	client, err := NewClient(testConfig)
	require.NoError(t, err)

	return client, cleanup
}
