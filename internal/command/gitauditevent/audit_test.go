package gitauditevent

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/sshenv"
)

var (
	testUsername = "gitlab-shell"
	testRepo     = "project-1"
	testKeyID    = 123
)

func TestGitAudit(t *testing.T) {
	tests := []struct {
		name        string
		keyID       int
		expectKeyID bool
	}{
		{name: "with deploy key", keyID: testKeyID, expectKeyID: true},
		{name: "without deploy key", keyID: 0, expectKeyID: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			requests := []testserver.TestRequestHandler{{
				Path: "/api/v4/internal/shellhorse/git_audit_event",
				Handler: func(w http.ResponseWriter, r *http.Request) {
					called = true

					requestBody := &struct {
						Action   commandargs.CommandType `json:"action"`
						Repo     string                  `json:"gl_repository"`
						Username string                  `json:"username"`
						KeyID    *int                    `json:"key_id"`
					}{}
					if !assert.NoError(t, testserver.DecodeJSON(r, requestBody)) {
						return
					}

					if tt.expectKeyID {
						if !assert.NotNil(t, requestBody.KeyID) {
							return
						}
						assert.Equal(t, tt.keyID, *requestBody.KeyID)
					} else {
						assert.Nil(t, requestBody.KeyID)
					}

					assert.Equal(t, testUsername, requestBody.Username)
					assert.Equal(t, testRepo, requestBody.Repo)

					w.WriteHeader(http.StatusOK)
				},
			}}

			args := &commandargs.Shell{
				CommandType: commandargs.UploadArchive,
				Env:         sshenv.Env{RemoteAddr: "18.245.0.42"},
			}

			url := testserver.StartSocketHTTPServer(t, requests)
			Audit(context.Background(), args, &config.Config{GitlabURL: url}, &accessverifier.Response{
				Username: testUsername,
				Repo:     testRepo,
				KeyID:    tt.keyID,
			}, nil)

			require.True(t, called)
		})
	}
}
