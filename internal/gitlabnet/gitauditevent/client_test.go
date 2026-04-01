package gitauditevent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pb "gitlab.com/gitlab-org/gitaly/v18/proto/go/gitalypb"
	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/sshenv"
)

var (
	testUsername            = "gitlab-shell"
	testKeyID               = 123
	testRepo                = "gitlab-org/gitlab-shell"
	testPackfileWants int64 = 100
	testPackfileHaves int64 = 100
	testArgs                = &commandargs.Shell{
		Env:         sshenv.Env{RemoteAddr: "18.245.0.42"},
		CommandType: "git-upload-pack",
	}
)

func TestAudit(t *testing.T) {
	tests := []struct {
		name        string
		keyID       int
		expectKeyID bool
	}{
		{
			name:        "with key_id",
			keyID:       testKeyID,
			expectKeyID: true,
		},
		{
			name:        "without key_id",
			keyID:       0,
			expectKeyID: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := setup(t, http.StatusOK, tt.keyID, tt.expectKeyID)

			err := client.Audit(context.Background(), AuditParams{
				Username: testUsername,
				KeyID:    tt.keyID,
				Repo:     testRepo,
				PackfileStats: &pb.PackfileNegotiationStatistics{
					Wants: testPackfileWants,
					Haves: testPackfileHaves,
				},
			}, testArgs)
			require.NoError(t, err)
		})
	}
}

func TestAuditFailed(t *testing.T) {
	client := setup(t, http.StatusBadRequest, testKeyID, true)

	err := client.Audit(context.Background(), AuditParams{
		Username: testUsername,
		KeyID:    testKeyID,
		Repo:     testRepo,
		PackfileStats: &pb.PackfileNegotiationStatistics{
			Wants: testPackfileWants,
			Haves: testPackfileHaves,
		},
	}, testArgs)
	require.Error(t, err)
}

func setup(t *testing.T, responseStatus int, keyID int, expectKeyID bool) *Client {
	requests := []testserver.TestRequestHandler{
		{
			Path: uri,
			Handler: func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				defer r.Body.Close()

				// Check if key_id is present/absent in raw JSON
				var rawJSON map[string]interface{}
				assert.NoError(t, json.Unmarshal(body, &rawJSON))
				_, hasKeyID := rawJSON["key_id"]
				if expectKeyID {
					assert.True(t, hasKeyID, "key_id should be present in JSON")
				} else {
					assert.False(t, hasKeyID, "key_id should not be present in JSON")
				}

				var request *Request
				assert.NoError(t, json.Unmarshal(body, &request))
				assert.Equal(t, testUsername, request.Username)
				assert.Equal(t, keyID, request.KeyID)
				assert.Equal(t, testArgs.Env.RemoteAddr, request.CheckIP)
				assert.Equal(t, testArgs.CommandType, request.Action)
				assert.Equal(t, testRepo, request.Repo)
				assert.Equal(t, "ssh", request.Protocol)
				assert.Equal(t, testPackfileWants, request.PackfileStats.Wants)
				assert.Equal(t, testPackfileHaves, request.PackfileStats.Haves)
				assert.Equal(t, "_any", request.Changes)

				w.WriteHeader(responseStatus)
			},
		},
	}

	url := testserver.StartSocketHTTPServer(t, requests)

	client, err := NewClient(&config.Config{GitlabURL: url})
	require.NoError(t, err)

	return client
}
