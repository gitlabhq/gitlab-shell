package discover

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet/testserver"
)

var (
	requests = []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/discover",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("key_id") == "1" || r.URL.Query().Get("username") == "alex-doe" {
					body := map[string]interface{}{
						"id":       2,
						"username": "alex-doe",
						"name":     "Alex Doe",
					}
					json.NewEncoder(w).Encode(body)
				} else if r.URL.Query().Get("username") == "broken_message" {
					body := map[string]string{
						"message": "Forbidden!",
					}
					w.WriteHeader(http.StatusForbidden)
					json.NewEncoder(w).Encode(body)
				} else if r.URL.Query().Get("username") == "broken" {
					w.WriteHeader(http.StatusInternalServerError)
				} else {
					fmt.Fprint(w, "null")
				}
			},
		},
	}
)

func TestExecute(t *testing.T) {
	url, cleanup := testserver.StartSocketHttpServer(t, requests)
	defer cleanup()

	testCases := []struct {
		desc           string
		arguments      *commandargs.Shell
		expectedOutput string
	}{
		{
			desc:           "With a known username",
			arguments:      &commandargs.Shell{GitlabUsername: "alex-doe"},
			expectedOutput: "Welcome to GitLab, @alex-doe!\n",
		},
		{
			desc:           "With a known key id",
			arguments:      &commandargs.Shell{GitlabKeyId: "1"},
			expectedOutput: "Welcome to GitLab, @alex-doe!\n",
		},
		{
			desc:           "With an unknown key",
			arguments:      &commandargs.Shell{GitlabKeyId: "-1"},
			expectedOutput: "Welcome to GitLab, Anonymous!\n",
		},
		{
			desc:           "With an unknown username",
			arguments:      &commandargs.Shell{GitlabUsername: "unknown"},
			expectedOutput: "Welcome to GitLab, Anonymous!\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			buffer := &bytes.Buffer{}
			cmd := &Command{
				Config:     &config.Config{GitlabUrl: url},
				Args:       tc.arguments,
				ReadWriter: &readwriter.ReadWriter{Out: buffer},
			}

			err := cmd.Execute()

			require.NoError(t, err)
			require.Equal(t, tc.expectedOutput, buffer.String())
		})
	}
}

func TestFailingExecute(t *testing.T) {
	url, cleanup := testserver.StartSocketHttpServer(t, requests)
	defer cleanup()

	testCases := []struct {
		desc          string
		arguments     *commandargs.Shell
		expectedError string
	}{
		{
			desc:          "With missing arguments",
			arguments:     &commandargs.Shell{},
			expectedError: "Failed to get username: who='' is invalid",
		},
		{
			desc:          "When the API returns an error",
			arguments:     &commandargs.Shell{GitlabUsername: "broken_message"},
			expectedError: "Failed to get username: Forbidden!",
		},
		{
			desc:          "When the API fails",
			arguments:     &commandargs.Shell{GitlabUsername: "broken"},
			expectedError: "Failed to get username: Internal API error (500)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			buffer := &bytes.Buffer{}
			cmd := &Command{
				Config:     &config.Config{GitlabUrl: url},
				Args:       tc.arguments,
				ReadWriter: &readwriter.ReadWriter{Out: buffer},
			}

			err := cmd.Execute()

			require.Empty(t, buffer.String())
			require.EqualError(t, err, tc.expectedError)
		})
	}
}
