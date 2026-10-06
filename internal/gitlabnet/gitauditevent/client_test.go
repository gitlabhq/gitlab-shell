package gitauditevent

import (
	"context"
	"encoding/json"
	"io"
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
	certArgs := *testArgs
	certArgs.Certificate = &commandargs.CertificateMetadata{
		CAFingerprint: "SHA256:ca-fingerprint",
		Identity:      "User@Example.com",
		TrustSource:   commandargs.CertificateTrustSourceGroup,
	}

	tests := []struct {
		name        string
		keyID       int
		expectKeyID bool
		args        *commandargs.Shell
	}{
		{
			name:        "with key_id",
			keyID:       testKeyID,
			expectKeyID: true,
			args:        testArgs,
		},
		{
			name:        "without key_id",
			keyID:       0,
			expectKeyID: false,
			args:        testArgs,
		},
		{
			name:        "with certificate metadata",
			keyID:       0,
			expectKeyID: false,
			args:        &certArgs,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := setup(t, http.StatusOK, tt.keyID, tt.expectKeyID, tt.args)

			err := client.Audit(context.Background(), AuditParams{
				Username: testUsername,
				KeyID:    tt.keyID,
				Repo:     testRepo,
				PackfileStats: &pb.PackfileNegotiationStatistics{
					Wants: testPackfileWants,
					Haves: testPackfileHaves,
				},
			}, tt.args)
			require.NoError(t, err)
		})
	}
}

func TestAuditFailed(t *testing.T) {
	client := setup(t, http.StatusBadRequest, testKeyID, true, testArgs)

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

func setup(t *testing.T, responseStatus int, keyID int, expectKeyID bool, args *commandargs.Shell) *Client {
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

				certFields := map[string]string{}
				if args.Certificate != nil {
					certFields = map[string]string{
						"ca_fingerprint":           args.Certificate.CAFingerprint,
						"certificate_identity":     args.Certificate.Identity,
						"certificate_trust_source": args.Certificate.TrustSource,
					}
				}
				for _, key := range []string{"ca_fingerprint", "certificate_identity", "certificate_trust_source"} {
					value, present := rawJSON[key]
					if expected, ok := certFields[key]; ok {
						assert.Equal(t, expected, value)
					} else {
						assert.False(t, present, "%s should not be present in JSON", key)
					}
				}

				var request *Request
				assert.NoError(t, json.Unmarshal(body, &request))
				assert.Equal(t, testUsername, request.Username)
				assert.Equal(t, keyID, request.KeyID)
				assert.Equal(t, args.Env.RemoteAddr, request.CheckIP)
				assert.Equal(t, args.CommandType, request.Action)
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
