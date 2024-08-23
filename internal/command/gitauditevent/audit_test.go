package gitauditevent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/gitauditevent"
)

var (
	testUsername = "gitlab-shell"
	testRepo     = "project-1"
)

func TestGitAudit(t *testing.T) {
	called := false

	requests := []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/shellhorse/git_audit_event",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				called = true

				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				defer r.Body.Close()

				var request *gitauditevent.Request
				require.NoError(t, json.Unmarshal(body, &request))
				require.Equal(t, testUsername, request.Username)
				require.Equal(t, testRepo, request.Repo)

				w.WriteHeader(http.StatusOK)
			},
		},
	}

	s := &commandargs.Shell{
		CommandType: commandargs.UploadArchive,
	}

	url := testserver.StartSocketHTTPServer(t, requests)
	Audit(context.Background(), s.CommandType, &config.Config{GitlabUrl: url}, &accessverifier.Response{
		Username: testUsername,
		Repo:     testRepo,
	}, nil)

	require.True(t, called)
}
