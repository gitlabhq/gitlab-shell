package authorizedkeys

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/gitlabnet/testserver"
)

var (
	requests = []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/authorized_keys",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("key") == "key" {
					body := map[string]interface{}{
						"id":  1,
						"key": "public-key",
					}
					json.NewEncoder(w).Encode(body)
				} else if r.URL.Query().Get("key") == "broken-message" {
					body := map[string]string{
						"message": "Forbidden!",
					}
					w.WriteHeader(http.StatusForbidden)
					json.NewEncoder(w).Encode(body)
				} else if r.URL.Query().Get("key") == "broken" {
					w.WriteHeader(http.StatusInternalServerError)
				} else {
					w.WriteHeader(http.StatusNotFound)
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
		arguments      []string
		expectedOutput string
	}{
		{
			desc:           "With matching username and key",
			arguments:      []string{"user", "user", "key"},
			expectedOutput: "command=\"/tmp/bin/gitlab-shell key-1\",no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty public-key\n",
		},
		{
			desc:          "When key doesn't match any existing key",
			arguments:     []string{"user", "user", "not-found"},
			expectedOutput: "# No key was found for not-found\n",
		},
		{
			desc:          "When the API returns an error",
			arguments:     []string{"user", "user", "broken-message"},
			expectedOutput: "# No key was found for broken-message\n",
		},
		{
			desc:          "When the API fails",
			arguments:     []string{"user", "user", "broken"},
			expectedOutput: "# No key was found for broken\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			buffer := &bytes.Buffer{}
			cmd := &Checker{
				Config:     &config.Config{RootDir: "/tmp", GitlabUrl: url},
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
		arguments     []string
		expectedError string
	}{
		{
			desc:          "With wrong number of arguments",
			arguments:     []string{"user"},
			expectedError: "# Wrong number of arguments. 1. Usage\n#\tgitlab-shell-authorized-keys-check <expected-username> <actual-username> <key>",
		},
		{
			desc:          "With missing username",
			arguments:     []string{"user", "", "key"},
			expectedError: "# No username provided",
		},
		{
			desc:          "With missing key",
			arguments:     []string{"user", "user", ""},
			expectedError: "# No key provided",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			buffer := &bytes.Buffer{}
			cmd := &Checker{
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
