package accessverifier

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tspb "gitlab.com/gitlab-org/cells/topology-service/clients/go/proto"
	pb "gitlab.com/gitlab-org/gitaly/v18/proto/go/gitalypb"
	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/sshenv"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/testhelper"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/topology"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/topology/topologytest"
)

var (
	namespace         = "group"
	repo              = "group/private"
	receivePackAction = commandargs.ReceivePack
	uploadPackAction  = commandargs.UploadPack
	defaultEnv        = sshenv.Env{NamespacePath: namespace}
)

func buildExpectedResponse(who string) *Response {
	response := &Response{
		Success:          true,
		UserID:           "user-1",
		Repo:             "project-26",
		Username:         "root",
		GitConfigOptions: []string{"option"},
		Gitaly: Gitaly{
			Repo: pb.Repository{
				StorageName:                   "default",
				RelativePath:                  "@hashed/5f/9c/5f9c4ab08cac7457e9111a30e4664920607ea2c115a1433d7be98e97e64244ca.git",
				GitObjectDirectory:            "path/to/git_object_directory",
				GitAlternateObjectDirectories: []string{"path/to/git_alternate_object_directory"},
				GlRepository:                  "project-26",
				GlProjectPath:                 repo,
			},
			Address: "unix:gitaly.socket",
			Token:   "token",
		},
		GitProtocol:     "protocol",
		Payload:         CustomPayload{},
		ConsoleMessages: []string{"console", "message"},
		Who:             who,
		StatusCode:      200,
	}

	return response
}

func TestSuccessfulResponses(t *testing.T) {
	testRoot := testhelper.PrepareTestRootDir(t)
	okResponse := testResponse{body: responseBody(t, testRoot, "allowed.json"), status: http.StatusOK}
	client := setup(t,
		map[string]testResponse{"first": okResponse, "test@TEST.TEST": okResponse},
		map[string]testResponse{"1": okResponse},
	)

	testCases := []struct {
		desc string
		args *commandargs.Shell
		who  string
	}{
		{
			desc: "Provide key id within the request",
			args: &commandargs.Shell{GitlabKeyID: "1"},
			who:  "key-1",
		}, {
			desc: "Provide username within the request",
			args: &commandargs.Shell{GitlabUsername: "first", Env: defaultEnv},
			who:  "user-1",
		}, {
			desc: "Provide krb5principal within the request",
			args: &commandargs.Shell{GitlabKrb5Principal: "test@TEST.TEST"},
			who:  "user-1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			result, err := client.Verify(context.Background(), tc.args, receivePackAction, repo)
			require.NoError(t, err)

			response := buildExpectedResponse(tc.who)
			require.Equal(t, response, result)
		})
	}
}

func TestGeoPushGetCustomAction(t *testing.T) {
	testRoot := testhelper.PrepareTestRootDir(t)
	client := setup(t, map[string]testResponse{
		"custom": {
			body:   responseBody(t, testRoot, "allowed_with_push_payload.json"),
			status: 300,
		},
	}, nil)

	args := &commandargs.Shell{GitlabUsername: "custom", Env: defaultEnv}
	result, err := client.Verify(context.Background(), args, receivePackAction, repo)
	require.NoError(t, err)

	response := buildExpectedResponse("user-1")
	response.Payload = CustomPayload{
		Action: "geo_proxy_to_primary",
		Data: CustomPayloadData{
			APIEndpoints:   []string{"geo/proxy_git_ssh/info_refs_receive_pack", "geo/proxy_git_ssh/receive_pack"},
			RequestHeaders: map[string]string{"Authorization": "Bearer token"},
			Username:       "custom",
			PrimaryRepo:    "https://repo/path",
		},
	}
	response.StatusCode = 300

	require.True(t, response.IsCustomAction())
	require.Equal(t, response, result)
}

func TestGeoPullGetCustomAction(t *testing.T) {
	testRoot := testhelper.PrepareTestRootDir(t)
	client := setup(t, map[string]testResponse{
		"custom": {
			body:   responseBody(t, testRoot, "allowed_with_pull_payload.json"),
			status: 300,
		},
	}, nil)

	args := &commandargs.Shell{GitlabUsername: "custom", Env: defaultEnv}
	result, err := client.Verify(context.Background(), args, uploadPackAction, repo)
	require.NoError(t, err)

	response := buildExpectedResponse("user-1")
	response.Payload = CustomPayload{
		Action: "geo_proxy_to_primary",
		Data: CustomPayloadData{
			APIEndpoints:   []string{"geo/proxy_git_ssh/info_refs_upload_pack", "geo/proxy_git_ssh/upload_pack"},
			Username:       "custom",
			PrimaryRepo:    "https://repo/path",
			RequestHeaders: map[string]string{"Authorization": "Bearer token"},
		},
	}
	response.StatusCode = 300

	require.True(t, response.IsCustomAction())
	require.Equal(t, response, result)
}

func TestErrorResponses(t *testing.T) {
	client := setup(t, nil, map[string]testResponse{
		"2": {body: []byte(`{"message":"Not allowed!"}`), status: http.StatusForbidden},
		"3": {body: []byte(`{"message":"broken json!`), status: http.StatusOK},
		"4": {status: http.StatusForbidden},
	})

	testCases := []struct {
		desc          string
		fakeID        string
		expectedError string
	}{
		{
			desc:          "A response with an error message",
			fakeID:        "2",
			expectedError: "Not allowed!",
		},
		{
			desc:          "A response with bad JSON",
			fakeID:        "3",
			expectedError: "parsing failed",
		},
		{
			desc:          "An error response without message",
			fakeID:        "4",
			expectedError: "Internal API error (403)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			args := &commandargs.Shell{GitlabKeyID: tc.fakeID}
			resp, err := client.Verify(context.Background(), args, receivePackAction, repo)

			require.EqualError(t, err, tc.expectedError)
			require.Nil(t, resp)
		})
	}
}

func TestCheckIP(t *testing.T) {
	testCases := []struct {
		desc            string
		remoteAddr      string
		expectedCheckIP string
	}{
		{
			desc:            "IPv4 address",
			remoteAddr:      "18.245.0.42",
			expectedCheckIP: "18.245.0.42",
		},
		{
			desc:            "IPv6 address",
			remoteAddr:      "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
			expectedCheckIP: "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
		},
		{
			desc:            "Host and port",
			remoteAddr:      "18.245.0.42:6345",
			expectedCheckIP: "18.245.0.42",
		},
		{
			desc:            "IPv6 host and port",
			remoteAddr:      "[2001:0db8:85a3:0000:0000:8a2e:0370:7334]:80",
			expectedCheckIP: "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
		},
		{
			desc:            "Bad remote addr",
			remoteAddr:      "[127.0",
			expectedCheckIP: "[127.0",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			client := setupWithAPIInspector(t,
				func(r *Request) {
					require.Equal(t, tc.expectedCheckIP, r.CheckIP)
				})

			sshEnv := sshenv.Env{RemoteAddr: tc.remoteAddr}
			client.Verify(context.Background(), &commandargs.Shell{Env: sshEnv}, uploadPackAction, repo)
		})
	}
}

type testResponse struct {
	body   []byte
	status int
}

func responseBody(t *testing.T, testRoot, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(path.Join(testRoot, "responses", name))
	require.NoError(t, err)
	return body
}

func setup(t *testing.T, userResponses, keyResponses map[string]testResponse) *Client {
	t.Helper()
	requests := []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/allowed",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				b, err := io.ReadAll(r.Body)
				assert.NoError(t, err)

				var requestBody *Request
				assert.NoError(t, json.Unmarshal(b, &requestBody))

				if tr, ok := userResponses[requestBody.Username]; ok {
					w.WriteHeader(tr.status)
					_, err := w.Write(tr.body)
					assert.NoError(t, err)
					assert.Equal(t, namespace, requestBody.NamespacePath)
				} else if tr, ok := userResponses[requestBody.Krb5Principal]; ok {
					w.WriteHeader(tr.status)
					_, err := w.Write(tr.body)
					assert.NoError(t, err)
					assert.Equal(t, sshProtocol, requestBody.Protocol)
				} else if tr, ok := keyResponses[requestBody.KeyID]; ok {
					w.WriteHeader(tr.status)
					_, err := w.Write(tr.body)
					assert.NoError(t, err)
					assert.Equal(t, sshProtocol, requestBody.Protocol)
				}
			},
		},
	}

	url := testserver.StartSocketHTTPServer(t, requests)

	client, err := NewClient(&config.Config{GitlabURL: url})
	require.NoError(t, err)

	return client
}

func TestVerifyWithTopologyService(t *testing.T) {
	testRoot := testhelper.PrepareTestRootDir(t)

	t.Run("routes /allowed to cell when TS returns PROXY", func(t *testing.T) {
		// Set up cell HTTP server
		var cellReceived bool
		cellServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cellReceived = true
			// Verify auth headers survive the WithHost swap
			assert.NotEmpty(t, r.Header.Get("Gitlab-Shell-Api-Request"), "JWT header must be present on cell request")
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			assert.NotEmpty(t, r.Header.Get("User-Agent"))
			w.Header().Set("Content-Type", "application/json")
			body := responseBody(t, testRoot, "allowed.json")
			_, _ = w.Write(body)
		}))
		t.Cleanup(cellServer.Close)

		// Set up default HTTP server (should NOT receive the request)
		var defaultReceived bool
		defaultServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			defaultReceived = true
			w.Header().Set("Content-Type", "application/json")
			body := responseBody(t, testRoot, "allowed.json")
			_, _ = w.Write(body)
		}))
		t.Cleanup(defaultServer.Close)

		// Set up mock TS that returns a PROXY action pointing to the cell.
		// Strip the scheme because the Resolver prepends it based on the GitLab URL.
		cellAddress := strings.TrimPrefix(cellServer.URL, "http://")
		mock := &topologytest.MockClassifyServer{
			Response: &tspb.ClassifyResponse{
				Action: tspb.ClassifyAction_PROXY,
				Proxy:  &tspb.ProxyInfo{Address: cellAddress},
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

		cfg := &config.Config{
			GitlabURL:      defaultServer.URL,
			Secret:         "test-secret",
			TopologyClient: tsClient,
		}

		client, err := NewClient(cfg)
		require.NoError(t, err)

		args := &commandargs.Shell{GitlabKeyID: "1"}
		result, err := client.Verify(context.Background(), args, receivePackAction, repo)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.True(t, result.Success)

		require.True(t, cellReceived, "request should have been sent to the cell server")
		require.False(t, defaultReceived, "request should NOT have been sent to the default server")
	})

	t.Run("falls back to default when TS is nil", func(t *testing.T) {
		var defaultReceived bool
		defaultServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			defaultReceived = true
			w.Header().Set("Content-Type", "application/json")
			body := responseBody(t, testRoot, "allowed.json")
			_, _ = w.Write(body)
		}))
		t.Cleanup(defaultServer.Close)

		cfg := &config.Config{
			GitlabURL:      defaultServer.URL,
			TopologyClient: nil,
		}

		client, err := NewClient(cfg)
		require.NoError(t, err)

		args := &commandargs.Shell{GitlabKeyID: "1"}
		result, err := client.Verify(context.Background(), args, receivePackAction, repo)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.True(t, result.Success)
		require.True(t, defaultReceived, "request should have been sent to the default server")
	})

	t.Run("falls back to default when TS returns error", func(t *testing.T) {
		var defaultReceived bool
		defaultServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			defaultReceived = true
			w.Header().Set("Content-Type", "application/json")
			body := responseBody(t, testRoot, "allowed.json")
			_, _ = w.Write(body)
		}))
		t.Cleanup(defaultServer.Close)

		mock := &topologytest.MockClassifyServer{
			Err: fmt.Errorf("TS unavailable"),
		}
		tsAddr, tsStop := topologytest.StartMockServer(t, mock)
		t.Cleanup(tsStop)

		tsClient := topology.NewClient(&topology.Config{
			Enabled: true,
			Address: tsAddr,
			Timeout: 5 * time.Second,
		})
		t.Cleanup(func() { _ = tsClient.Close() })

		cfg := &config.Config{
			GitlabURL:      defaultServer.URL,
			TopologyClient: tsClient,
		}

		client, err := NewClient(cfg)
		require.NoError(t, err)

		args := &commandargs.Shell{GitlabKeyID: "1"}
		result, err := client.Verify(context.Background(), args, receivePackAction, repo)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.True(t, result.Success)
		require.True(t, defaultReceived, "request should have fallen back to the default server")
	})
}

func setupWithAPIInspector(t *testing.T, inspector func(*Request)) *Client {
	t.Helper()
	requests := []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/allowed",
			Handler: func(_ http.ResponseWriter, r *http.Request) {
				b, err := io.ReadAll(r.Body)
				assert.NoError(t, err)

				var requestBody *Request
				err = json.Unmarshal(b, &requestBody)
				assert.NoError(t, err)

				inspector(requestBody)
			},
		},
	}

	url := testserver.StartSocketHTTPServer(t, requests)

	client, err := NewClient(&config.Config{GitlabURL: url})
	require.NoError(t, err)

	return client
}
