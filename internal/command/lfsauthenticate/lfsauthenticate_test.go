package lfsauthenticate

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet/lfsauthenticate"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/internal/testhelper/requesthandlers"
)

func TestFailedRequests(t *testing.T) {
	requests := requesthandlers.BuildDisallowedByApiHandlers(t)
	url, cleanup := testserver.StartHttpServer(t, requests)
	defer cleanup()

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
			arguments:      &commandargs.Shell{GitlabKeyId: "1", SshArgs: []string{"git-lfs-authenticate", "group/repo", "unknown"}},
			expectedOutput: "Disallowed command",
		},
		{
			desc:           "With disallowed user",
			arguments:      &commandargs.Shell{GitlabKeyId: "disallowed", SshArgs: []string{"git-lfs-authenticate", "group/repo", "download"}},
			expectedOutput: "Disallowed by API call",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			output := &bytes.Buffer{}
			cmd := &Command{
				Config:     &config.Config{GitlabUrl: url},
				Args:       tc.arguments,
				ReadWriter: &readwriter.ReadWriter{ErrOut: output, Out: output},
			}

			err := cmd.Execute()
			require.Error(t, err)

			require.Equal(t, tc.expectedOutput, err.Error())
		})
	}
}

func TestLfsAuthenticateRequests(t *testing.T) {
	userId := "123"
	operation := "upload"

	requests := []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/lfs_authenticate",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				b, err := ioutil.ReadAll(r.Body)
				defer r.Body.Close()
				require.NoError(t, err)

				var request *lfsauthenticate.Request
				require.NoError(t, json.Unmarshal(b, &request))
				require.Equal(t, request.Operation, operation)

				if request.UserId == userId {
					body := map[string]interface{}{
						"username":             "john",
						"lfs_token":            "sometoken",
						"repository_http_path": "https://gitlab.com/repo/path",
						"expires_in":           1800,
					}
					require.NoError(t, json.NewEncoder(w).Encode(body))
				} else {
					w.WriteHeader(http.StatusForbidden)
				}
			},
		},
		{
			Path: "/api/v4/internal/allowed",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				b, err := ioutil.ReadAll(r.Body)
				defer r.Body.Close()
				require.NoError(t, err)

				var request *accessverifier.Request
				require.NoError(t, json.Unmarshal(b, &request))

				var glId string
				if request.Username == "somename" {
					glId = userId
				} else {
					glId = "100"
				}

				body := map[string]interface{}{
					"gl_id":  glId,
					"status": true,
				}
				require.NoError(t, json.NewEncoder(w).Encode(body))
			},
		},
	}

	url, cleanup := testserver.StartHttpServer(t, requests)
	defer cleanup()

	testCases := []struct {
		desc           string
		username       string
		expectedOutput string
	}{
		{
			desc:           "With successful response from API",
			username:       "somename",
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
				Config:     &config.Config{GitlabUrl: url},
				Args:       &commandargs.Shell{GitlabUsername: tc.username, SshArgs: []string{"git-lfs-authenticate", "group/repo", operation}},
				ReadWriter: &readwriter.ReadWriter{ErrOut: output, Out: output},
			}

			err := cmd.Execute()
			require.NoError(t, err)

			require.Equal(t, tc.expectedOutput, output.String())
		})
	}
}
