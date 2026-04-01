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
	client := setup(t, http.StatusOK)

	err := client.Audit(context.Background(), AuditParams{
		Username: testUsername,
		KeyID:    testKeyID,
		Repo:     testRepo,
		PackfileStats: &pb.PackfileNegotiationStatistics{
			Wants: testPackfileWants,
			Haves: testPackfileHaves,
		},
	}, testArgs)
	require.NoError(t, err)
}

func TestAuditFailed(t *testing.T) {
	client := setup(t, http.StatusBadRequest)

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

func TestAuditWithoutKeyID(t *testing.T) {
	requests := []testserver.TestRequestHandler{
		{
			Path: uri,
			Handler: func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				defer r.Body.Close()

				// Verify key_id is not present in JSON when KeyID is 0
				var rawJSON map[string]interface{}
				assert.NoError(t, json.Unmarshal(body, &rawJSON))
				_, hasKeyID := rawJSON["key_id"]
				assert.False(t, hasKeyID, "key_id should not be present in JSON when KeyID is 0")

				// Verify other fields are still present
				var request *Request
				assert.NoError(t, json.Unmarshal(body, &request))
				assert.Equal(t, testUsername, request.Username)
				assert.Equal(t, testRepo, request.Repo)

				w.WriteHeader(http.StatusOK)
			},
		},
	}

	url := testserver.StartSocketHTTPServer(t, requests)
	client, err := NewClient(&config.Config{GitlabURL: url})
	require.NoError(t, err)

	err = client.Audit(context.Background(), AuditParams{
		Username: testUsername,
		KeyID:    0, // No key ID (e.g., Kerberos auth)
		Repo:     testRepo,
		PackfileStats: &pb.PackfileNegotiationStatistics{
			Wants: testPackfileWants,
			Haves: testPackfileHaves,
		},
	}, testArgs)
	require.NoError(t, err)
}

func setup(t *testing.T, responseStatus int) *Client {
	requests := []testserver.TestRequestHandler{
		{
			Path: uri,
			Handler: func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				defer r.Body.Close()

				var request *Request
				assert.NoError(t, json.Unmarshal(body, &request))
				assert.Equal(t, testUsername, request.Username)
				assert.Equal(t, testKeyID, request.KeyID)
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
