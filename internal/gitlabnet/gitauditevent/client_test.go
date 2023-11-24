package gitauditevent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	pb "gitlab.com/gitlab-org/gitaly/v16/proto/go/gitalypb"
	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
)

var (
	testUsername                              = "gitlab-shell"
	testAction        commandargs.CommandType = "git-upload-pack"
	testRepo                                  = "gitlab-org/gitlab-shell"
	testPackfileWants int64                   = 100
	testPackfileHaves int64                   = 100
)

func TestAudit(t *testing.T) {
	client := setup(t, http.StatusOK)

	err := client.Audit(context.Background(), testUsername, testAction, testRepo, &pb.PackfileNegotiationStatistics{
		Wants: testPackfileWants,
		Haves: testPackfileHaves,
	})
	require.NoError(t, err)
}

func TestAuditFailed(t *testing.T) {
	client := setup(t, http.StatusBadRequest)

	err := client.Audit(context.Background(), testUsername, testAction, testRepo, &pb.PackfileNegotiationStatistics{
		Wants: testPackfileWants,
		Haves: testPackfileHaves,
	})
	require.Error(t, err)
}

func setup(t *testing.T, responseStatus int) *Client {
	requests := []testserver.TestRequestHandler{
		{
			Path: uri,
			Handler: func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				defer r.Body.Close()

				var request *Request
				require.NoError(t, json.Unmarshal(body, &request))
				require.Equal(t, testUsername, request.Username)
				require.Equal(t, testAction, request.Action)
				require.Equal(t, testRepo, request.Repo)
				require.Equal(t, "ssh", request.Protocol)
				require.Equal(t, testPackfileWants, request.PackfileStats.Wants)
				require.Equal(t, testPackfileHaves, request.PackfileStats.Haves)

				w.WriteHeader(responseStatus)
			},
		},
	}

	url := testserver.StartSocketHttpServer(t, requests)

	client, err := NewClient(&config.Config{GitlabUrl: url})
	require.NoError(t, err)

	return client
}
