package lfsauthenticate

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tspb "gitlab.com/gitlab-org/cells/topology-service/clients/go/proto"
	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/lfsauthenticate"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/testhelper/requesthandlers"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/topology"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/topology/topologytest"
)

const (
	testLFSAuthenticate = "git-lfs-authenticate"
	testSomename        = "somename"
	testRepo            = "group/repo"
)

func TestFailedRequests(t *testing.T) {
	requests := requesthandlers.BuildDisallowedByAPIHandlers(t)
	url := testserver.StartHTTPServer(t, requests)

	testCases := []struct {
		desc           string
		arguments      *commandargs.Shell
		expectedOutput string
	}{
		{
			desc:           "With missing arguments",
			arguments:      &commandargs.Shell{},
			expectedOutput: "Disallowed command",
		},
		{
			desc:           "With disallowed command",
			arguments:      &commandargs.Shell{GitlabKeyID: "1", SSHArgs: []string{testLFSAuthenticate, testRepo, "unknown"}},
			expectedOutput: "Disallowed command",
		},
		{
			desc:           "With disallowed user",
			arguments:      &commandargs.Shell{GitlabKeyID: "disallowed", SSHArgs: []string{testLFSAuthenticate, testRepo, "download"}},
			expectedOutput: "Disallowed by API call",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			output := &bytes.Buffer{}
			cmd := &Command{
				Config:     &config.Config{GitlabURL: url},
				Args:       tc.arguments,
				ReadWriter: &readwriter.ReadWriter{ErrOut: output, Out: output},
			}

			_, err := cmd.Execute(context.Background())
			require.Error(t, err)

			require.Equal(t, tc.expectedOutput, err.Error())
		})
	}
}

func TestLfsAuthenticateRequests(t *testing.T) {
	glID := "user-123"
	operation := "upload"

	requests := []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/lfs_authenticate",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				b, err := io.ReadAll(r.Body)
				defer r.Body.Close()
				assert.NoError(t, err)

				var request *lfsauthenticate.Request
				assert.NoError(t, json.Unmarshal(b, &request))
				assert.Equal(t, request.Operation, operation)

				if request.UserID == "123" {
					body := map[string]interface{}{
						"username":             "john",
						"lfs_token":            "sometoken",
						"repository_http_path": "https://gitlab.com/repo/path",
						"expires_in":           1800,
					}
					assert.NoError(t, json.NewEncoder(w).Encode(body))
				} else {
					w.WriteHeader(http.StatusForbidden)
				}
			},
		},
		{
			Path: "/api/v4/internal/allowed",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				b, err := io.ReadAll(r.Body)
				defer r.Body.Close()
				assert.NoError(t, err)

				var request *accessverifier.Request
				assert.NoError(t, json.Unmarshal(b, &request))

				var responseGlID string
				if request.Username == testSomename {
					responseGlID = glID
				} else {
					responseGlID = "100"
				}

				body := map[string]interface{}{
					"gl_id":       responseGlID,
					"status":      true,
					"gl_username": "alex-doe",
					"gitaly": map[string]interface{}{
						"repository": map[string]interface{}{
							"gl_project_path": "group/project-path",
						},
					},
				}
				assert.NoError(t, json.NewEncoder(w).Encode(body))
			},
		},
	}

	url := testserver.StartHTTPServer(t, requests)

	testCases := []struct {
		desc           string
		username       string
		expectedOutput string
	}{
		{
			desc:           "With successful response from API",
			username:       testSomename,
			expectedOutput: "{\"header\":{\"Authorization\":\"Basic am9objpzb21ldG9rZW4=\"},\"href\":\"https://gitlab.com/repo/path/info/lfs\",\"expires_in\":1800}\n",
		},
		{
			desc:           "With forbidden response from API",
			username:       "anothername",
			expectedOutput: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			output := &bytes.Buffer{}
			cmd := &Command{
				Config:     &config.Config{GitlabURL: url},
				Args:       &commandargs.Shell{GitlabUsername: tc.username, SSHArgs: []string{testLFSAuthenticate, testRepo, operation}},
				ReadWriter: &readwriter.ReadWriter{ErrOut: output, Out: output},
			}

			ctxWithLogData, err := cmd.Execute(context.Background())

			require.NoError(t, err)
			require.Equal(t, tc.expectedOutput, output.String())

			data := ctxWithLogData.Value(logInfo{}).(command.LogData)
			require.Equal(t, "alex-doe", data.Username)
			require.Equal(t, "group/project-path", data.Meta.Project)
			require.Equal(t, "group", data.Meta.RootNamespace)
		})
	}
}

func TestLfsAuthenticateWithTopologyService(t *testing.T) {
	// Create a "cell" HTTP server that handles both /allowed and /lfs_authenticate
	var cellAllowedReceived, cellLfsReceived bool
	cellServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/allowed"):
			cellAllowedReceived = true
			body := map[string]interface{}{
				"gl_id":       "user-123",
				"status":      true,
				"gl_username": "alex-doe",
				"gitaly": map[string]interface{}{
					"repository": map[string]interface{}{
						"gl_project_path": "group/project-path",
					},
				},
			}
			assert.NoError(t, json.NewEncoder(w).Encode(body))
		case strings.HasSuffix(r.URL.Path, "/lfs_authenticate"):
			cellLfsReceived = true
			body := map[string]interface{}{
				"username":             "john",
				"lfs_token":            "sometoken",
				"repository_http_path": "https://gitlab.com/repo/path",
				"expires_in":           1800,
			}
			assert.NoError(t, json.NewEncoder(w).Encode(body))
		}
	}))
	t.Cleanup(cellServer.Close)

	// Create a "default" HTTP server that should NOT receive any requests
	defaultServer := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("default server unexpectedly received request: %s %s", r.Method, r.URL.Path)
	}))
	t.Cleanup(defaultServer.Close)

	// Create mock Topology Service returning PROXY -> cell
	tsProxyAddr := strings.TrimPrefix(cellServer.URL, "http://")
	mock := &topologytest.MockClassifyServer{
		Response: &tspb.ClassifyResponse{
			Action: tspb.ClassifyAction_PROXY,
			Proxy:  &tspb.ProxyInfo{Address: tsProxyAddr},
		},
	}
	tsAddr, tsStop := topologytest.StartMockServer(t, mock)
	t.Cleanup(tsStop)

	tsClient := topology.NewClient(&topology.Config{
		Enabled: true,
		Address: tsAddr,
		Timeout: 5 * time.Second,
	})
	t.Cleanup(func() { _ = tsClient.Close() })

	// Build config pointing at default server but with TS routing to cell
	cfg := &config.Config{
		GitlabURL:      defaultServer.URL,
		Secret:         "test-secret",
		TopologyClient: tsClient,
	}

	// Execute the LFS authenticate command
	output := &bytes.Buffer{}
	cmd := &Command{
		Config: cfg,
		Args: &commandargs.Shell{
			GitlabUsername: testSomename,
			SSHArgs:        []string{testLFSAuthenticate, testRepo, "upload"},
		},
		ReadWriter: &readwriter.ReadWriter{ErrOut: output, Out: output},
	}

	ctxWithLogData, err := cmd.Execute(context.Background())
	require.NoError(t, err)

	// Verify: both /allowed and /lfs_authenticate hit the cell, not the default
	require.True(t, cellAllowedReceived, "/allowed should have been sent to the cell server")
	require.True(t, cellLfsReceived, "/lfs_authenticate should have been sent to the cell server")

	// Verify the output is valid LFS payload
	require.Contains(t, output.String(), "\"href\":")
	require.Contains(t, output.String(), "\"header\":")

	// Verify the context was enriched with log data through the routed path
	data := ctxWithLogData.Value(logInfo{}).(command.LogData)
	require.Equal(t, "alex-doe", data.Username)
	require.Equal(t, "group/project-path", data.Meta.Project)
	require.Equal(t, "group", data.Meta.RootNamespace)
}
