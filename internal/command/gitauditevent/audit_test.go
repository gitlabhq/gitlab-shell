package gitauditevent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/gitauditevent"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/sshenv"
)

var (
	testUsername = "gitlab-shell"
	testRepo     = "project-1"
	testKeyID    = 123
)

func TestGitAudit(t *testing.T) {
	called := false

	requests := []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/shellhorse/git_audit_event",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				called = true

				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				defer r.Body.Close()

				// Verify key_id is present in JSON
				var rawJSON map[string]interface{}
				assert.NoError(t, json.Unmarshal(body, &rawJSON))
				keyID, hasKeyID := rawJSON["key_id"]
				assert.True(t, hasKeyID, "key_id should be present in JSON when KeyID is set")
				assert.Equal(t, testKeyID, int(keyID.(float64)))

				var request *gitauditevent.Request
				assert.NoError(t, json.Unmarshal(body, &request))
				assert.Equal(t, testUsername, request.Username)
				assert.Equal(t, testRepo, request.Repo)

				w.WriteHeader(http.StatusOK)
			},
		},
	}

	args := &commandargs.Shell{
		CommandType: commandargs.UploadArchive,
		Env:         sshenv.Env{RemoteAddr: "18.245.0.42"},
	}

	url := testserver.StartSocketHTTPServer(t, requests)
	Audit(context.Background(), args, &config.Config{GitlabURL: url}, &accessverifier.Response{
		Username: testUsername,
		Repo:     testRepo,
		KeyID:    testKeyID,
	}, nil)

	require.True(t, called)
}

func TestGitAuditWithoutKeyID(t *testing.T) {
	called := false

	requests := []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/shellhorse/git_audit_event",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				called = true

				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				defer r.Body.Close()

				// Verify key_id is NOT present in JSON when KeyID is 0
				var rawJSON map[string]interface{}
				assert.NoError(t, json.Unmarshal(body, &rawJSON))
				_, hasKeyID := rawJSON["key_id"]
				assert.False(t, hasKeyID, "key_id should not be present in JSON when KeyID is 0")

				w.WriteHeader(http.StatusOK)
			},
		},
	}

	args := &commandargs.Shell{
		CommandType: commandargs.UploadArchive,
		Env:         sshenv.Env{RemoteAddr: "18.245.0.42"},
	}

	url := testserver.StartSocketHTTPServer(t, requests)
	Audit(context.Background(), args, &config.Config{GitlabURL: url}, &accessverifier.Response{
		Username: testUsername,
		Repo:     testRepo,
		KeyID:    0, // No key ID (e.g., Kerberos auth)
	}, nil)

	require.True(t, called)
}
