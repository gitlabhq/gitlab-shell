package discover

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
)

var requests = []testserver.TestRequestHandler{
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

func TestExecute(t *testing.T) {
	url := testserver.StartSocketHttpServer(t, requests)

	testCases := []struct {
		desc             string
		arguments        *commandargs.Shell
		expectedUsername string
	}{
		{
			desc:             "With a known username",
			arguments:        &commandargs.Shell{GitlabUsername: "alex-doe"},
			expectedUsername: "@alex-doe",
		},
		{
			desc:             "With a known key id",
			arguments:        &commandargs.Shell{GitlabKeyId: "1"},
			expectedUsername: "@alex-doe",
		},
		{
			desc:             "With an unknown key",
			arguments:        &commandargs.Shell{GitlabKeyId: "-1"},
			expectedUsername: "Anonymous",
		},
		{
			desc:             "With an unknown username",
			arguments:        &commandargs.Shell{GitlabUsername: "unknown"},
			expectedUsername: "Anonymous",
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

			ctxWithLogMetadata, err := cmd.Execute(context.Background())

			expectedOutput := fmt.Sprintf("Welcome to GitLab, %s!\n", tc.expectedUsername)
			expectedUsername := strings.TrimLeft(tc.expectedUsername, "@")

			require.NoError(t, err)
			require.Equal(t, expectedOutput, buffer.String())
			require.Equal(t, expectedUsername, ctxWithLogMetadata.Value("metadata").(command.LogMetadata).Username)
		})
	}
}

func TestFailingExecute(t *testing.T) {
	url := testserver.StartSocketHttpServer(t, requests)

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
			expectedError: "Failed to get username: Internal API unreachable",
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

			_, err := cmd.Execute(context.Background())

			require.Empty(t, buffer.String())
			require.EqualError(t, err, tc.expectedError)
		})
	}
}
