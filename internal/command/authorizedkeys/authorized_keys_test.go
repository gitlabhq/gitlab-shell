package authorizedkeys

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/testhelper/retryopts"
)

const (
	testKey  = "key"
	testUser = "user"
)

var (
	requests = []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/authorized_keys",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("key") == testKey {
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
	url := testserver.StartSocketHTTPServer(t, requests)

	defaultConfig := &config.Config{RootDir: "/tmp", GitlabURL: url, HTTPClientOpts: retryopts.FastRetryOpts()}

	testCases := []struct {
		desc           string
		arguments      *commandargs.AuthorizedKeys
		expectedOutput string
	}{
		{
			desc:           "With matching username and key",
			arguments:      &commandargs.AuthorizedKeys{ExpectedUser: testUser, ActualUser: testUser, Key: testKey},
			expectedOutput: "command=\"/tmp/bin/gitlab-shell key-1\",no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty public-key\n",
		},
		{
			desc:           "When key doesn't match any existing key",
			arguments:      &commandargs.AuthorizedKeys{ExpectedUser: testUser, ActualUser: testUser, Key: "not-found"},
			expectedOutput: "# No key was found for not-found\n",
		},
		{
			desc:           "When the API returns an error",
			arguments:      &commandargs.AuthorizedKeys{ExpectedUser: testUser, ActualUser: testUser, Key: "broken-message"},
			expectedOutput: "# No key was found for broken-message\n",
		},
		{
			desc:           "When the API fails",
			arguments:      &commandargs.AuthorizedKeys{ExpectedUser: testUser, ActualUser: testUser, Key: "broken"},
			expectedOutput: "# No key was found for broken\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			buffer := &bytes.Buffer{}

			cmd := &Command{
				Config:     defaultConfig,
				Args:       tc.arguments,
				ReadWriter: &readwriter.ReadWriter{Out: buffer},
			}

			_, err := cmd.Execute(context.Background())

			require.NoError(t, err)
			require.Equal(t, tc.expectedOutput, buffer.String())
		})
	}
}
