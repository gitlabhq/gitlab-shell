package authorizedkeys

import (
	"bytes"
	"encoding/json"
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
		arguments      *commandargs.AuthorizedKeys
		expectedOutput string
	}{
		{
			desc:           "With matching username and key",
			arguments:      &commandargs.AuthorizedKeys{ExpectedUser: "user", ActualUser: "user", Key: "key"},
			expectedOutput: "command=\"/tmp/bin/gitlab-shell key-1\",no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty public-key\n",
		},
		{
			desc:           "When key doesn't match any existing key",
			arguments:      &commandargs.AuthorizedKeys{ExpectedUser: "user", ActualUser: "user", Key: "not-found"},
			expectedOutput: "# No key was found for not-found\n",
		},
		{
			desc:           "When the API returns an error",
			arguments:      &commandargs.AuthorizedKeys{ExpectedUser: "user", ActualUser: "user", Key: "broken-message"},
			expectedOutput: "# No key was found for broken-message\n",
		},
		{
			desc:           "When the API fails",
			arguments:      &commandargs.AuthorizedKeys{ExpectedUser: "user", ActualUser: "user", Key: "broken"},
			expectedOutput: "# No key was found for broken\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			buffer := &bytes.Buffer{}
			cmd := &Command{
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
