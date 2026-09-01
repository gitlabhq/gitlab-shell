package gitauditevent

import (
	"context"
	"net/http"
	"net/http/httptest"
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
	testNamespacePath       = "gitlab-org"
	testArgs                = &commandargs.Shell{
		Env:         sshenv.Env{RemoteAddr: "18.245.0.42", NamespacePath: testNamespacePath},
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
	type requestBody struct {
		Action        commandargs.CommandType           `json:"action"`
		Protocol      string                            `json:"protocol"`
		Repo          string                            `json:"gl_repository"`
		Username      string                            `json:"username"`
		KeyID         *int                              `json:"key_id"`
		PackfileStats *pb.PackfileNegotiationStatistics `json:"packfile_stats"`
		CheckIP       string                            `json:"check_ip"`
		Changes       string                            `json:"changes"`
		NamespacePath string                            `json:"namespace_path"`
	}

	requests := []testserver.TestRequestHandler{
		{
			Path: uri,
			Handler: func(w http.ResponseWriter, r *http.Request) {
				request := &requestBody{}
				if !assert.NoError(t, testserver.DecodeJSON(r, request)) {
					http.Error(w, "invalid request", http.StatusBadRequest)
					return
				}

				if expectKeyID {
					assert.Equal(t, &keyID, request.KeyID)
				} else {
					assert.Nil(t, request.KeyID)
				}

				assert.Equal(t, testUsername, request.Username)
				assert.Equal(t, testArgs.Env.RemoteAddr, request.CheckIP)
				assert.Equal(t, testArgs.CommandType, request.Action)
				assert.Equal(t, testRepo, request.Repo)
				assert.Equal(t, "ssh", request.Protocol)
				assert.Equal(t, testPackfileWants, request.PackfileStats.Wants)
				assert.Equal(t, testPackfileHaves, request.PackfileStats.Haves)
				assert.Equal(t, "_any", request.Changes)
				assert.Equal(t, testNamespacePath, request.NamespacePath)

				w.WriteHeader(responseStatus)
			},
		},
	}

	url := testserver.StartSocketHTTPServer(t, requests)

	client, err := NewClient(&config.Config{GitlabURL: url})
	require.NoError(t, err)

	return client
}

func TestAuditWithCellAddress(t *testing.T) {
	var cellReceived bool
	cellServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		cellReceived = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(cellServer.Close)

	var defaultReceived bool
	defaultServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		defaultReceived = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(defaultServer.Close)

	client, err := NewClient(&config.Config{GitlabURL: defaultServer.URL})
	require.NoError(t, err)

	err = client.Audit(context.Background(), AuditParams{
		Username:    testUsername,
		KeyID:       testKeyID,
		Repo:        testRepo,
		CellAddress: cellServer.URL,
	}, testArgs)
	require.NoError(t, err)

	require.True(t, cellReceived, "request should have been sent to the cell server")
	require.False(t, defaultReceived, "request should NOT have been sent to the default server")
}
