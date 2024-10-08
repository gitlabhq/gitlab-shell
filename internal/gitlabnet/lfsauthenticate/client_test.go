package lfsauthenticate

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
)

const (
	keyID = "123"
	repo  = "group/repo"
)

func setup(t *testing.T) []testserver.TestRequestHandler {
	requests := []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/lfs_authenticate",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				b, err := io.ReadAll(r.Body)
				defer r.Body.Close()
				assert.NoError(t, err)

				var request *Request
				assert.NoError(t, json.Unmarshal(b, &request))

				switch request.KeyID {
				case keyID:
					body := map[string]interface{}{
						"username":             "john",
						"lfs_token":            "sometoken",
						"repository_http_path": "https://gitlab.com/repo/path",
						"expires_in":           1800,
					}
					assert.NoError(t, json.NewEncoder(w).Encode(body))
				case "forbidden":
					w.WriteHeader(http.StatusForbidden)
				case "broken":
					w.WriteHeader(http.StatusInternalServerError)
				}
			},
		},
	}

	return requests
}

func TestFailedRequests(t *testing.T) {
	requests := setup(t)
	url := testserver.StartHTTPServer(t, requests)

	testCases := []struct {
		desc           string
		args           *commandargs.Shell
		expectedOutput string
	}{
		{
			desc:           "With bad response",
			args:           &commandargs.Shell{GitlabKeyID: "-1", CommandType: commandargs.LfsAuthenticate, SSHArgs: []string{"git-lfs-authenticate", repo, "download"}},
			expectedOutput: "parsing failed",
		},
		{
			desc:           "With API returns an error",
			args:           &commandargs.Shell{GitlabKeyID: "forbidden", CommandType: commandargs.LfsAuthenticate, SSHArgs: []string{"git-lfs-authenticate", repo, "download"}},
			expectedOutput: "Internal API error (403)",
		},
		{
			desc:           "With API fails",
			args:           &commandargs.Shell{GitlabKeyID: "broken", CommandType: commandargs.LfsAuthenticate, SSHArgs: []string{"git-lfs-authenticate", repo, "download"}},
			expectedOutput: "Internal API unreachable",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			client, err := NewClient(&config.Config{GitlabUrl: url}, tc.args)
			require.NoError(t, err)

			operation := tc.args.SSHArgs[2]

			_, err = client.Authenticate(context.Background(), operation, repo, "")
			require.Error(t, err)

			assert.Equal(t, tc.expectedOutput, err.Error())
		})
	}
}

func TestSuccessfulRequests(t *testing.T) {
	requests := setup(t)
	url := testserver.StartHTTPServer(t, requests)

	testCases := []struct {
		desc      string
		operation string
	}{
		{
			desc:      "For download",
			operation: "download",
		},
		{
			desc:      "For upload",
			operation: "upload",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			operation := tc.operation
			args := &commandargs.Shell{GitlabKeyID: keyID, CommandType: commandargs.LfsAuthenticate, SSHArgs: []string{"git-lfs-authenticate", repo, operation}}
			client, err := NewClient(&config.Config{GitlabUrl: url}, args)
			require.NoError(t, err)

			response, err := client.Authenticate(context.Background(), operation, repo, "")
			require.NoError(t, err)

			expectedResponse := &Response{
				Username:  "john",
				LfsToken:  "sometoken",
				RepoPath:  "https://gitlab.com/repo/path",
				ExpiresIn: 1800,
			}

			assert.Equal(t, expectedResponse, response)
		})
	}
}
