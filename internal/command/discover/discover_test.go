package discover

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/labkit/v2/log"
)

const (
	testUsername   = "alex-doe"
	testUsernameAt = "@alex-doe"

	brokenUsername      = "broken_message"
	brokenKeyID         = "2"
	brokenKrb5Principal = "broken@TEST.TEST"
	unreachableUsername = "broken"
	forbiddenError      = "Failed to get username: Forbidden!"

	clientErrorMsg = "Internal API returned a client error"
	apiErrorMsg    = "Internal API error"

	userNameField      = "gl_user_name"
	keyIDField         = "gl_key_id"
	krb5PrincipalField = "krb5_principal"
)

var requests = []testserver.TestRequestHandler{
	{
		Path: "/api/v4/internal/discover",
		Handler: func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("key_id") == "1" || r.URL.Query().Get("username") == testUsername {
				body := map[string]interface{}{
					"id":       2,
					"username": testUsername,
					"name":     "Alex Doe",
				}
				json.NewEncoder(w).Encode(body)
			} else if r.URL.Query().Get("username") == brokenUsername ||
				r.URL.Query().Get("key_id") == brokenKeyID ||
				r.URL.Query().Get("krb5principal") == brokenKrb5Principal {
				body := map[string]string{
					"message": "Forbidden!",
				}
				w.WriteHeader(http.StatusForbidden)
				json.NewEncoder(w).Encode(body)
			} else if r.URL.Query().Get("username") == unreachableUsername {
				w.WriteHeader(http.StatusInternalServerError)
			} else {
				fmt.Fprint(w, "null")
			}
		},
	},
}

func TestExecute(t *testing.T) {
	url := testserver.StartSocketHTTPServer(t, requests)

	testCases := []struct {
		desc             string
		arguments        *commandargs.Shell
		expectedUsername string
	}{
		{
			desc:             "With a known username",
			arguments:        &commandargs.Shell{GitlabUsername: testUsername},
			expectedUsername: testUsernameAt,
		},
		{
			desc:             "With a known key id",
			arguments:        &commandargs.Shell{GitlabKeyID: "1"},
			expectedUsername: testUsernameAt,
		},
		{
			desc:             "With an unknown key",
			arguments:        &commandargs.Shell{GitlabKeyID: "-1"},
			expectedUsername: "Anonymous",
		},
		{
			desc:             "With an unknown username",
			arguments:        &commandargs.Shell{GitlabUsername: "unknown"},
			expectedUsername: "Anonymous",
		},
		{
			desc: "with a known username - when other input is also given",
			arguments: &commandargs.Shell{
				Arguments:      []string{"foo"},
				GitlabUsername: testUsername,
			},
			expectedUsername: testUsernameAt,
		},
		{
			desc: "with a known key id - when other input is also given",
			arguments: &commandargs.Shell{
				Arguments:   []string{"2foo"},
				GitlabKeyID: "1",
			},
			expectedUsername: testUsernameAt,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			buffer := &bytes.Buffer{}
			cmd := &Command{
				Config:     &config.Config{GitlabURL: url},
				Args:       tc.arguments,
				ReadWriter: &readwriter.ReadWriter{Out: buffer},
			}

			ctxWithLogData, err := cmd.Execute(context.Background())

			expectedOutput := fmt.Sprintf("Welcome to GitLab, %s!\n", tc.expectedUsername)
			expectedUsername := strings.TrimLeft(tc.expectedUsername, "@")

			require.NoError(t, err)
			require.Equal(t, expectedOutput, buffer.String())
			require.Equal(t, expectedUsername, ctxWithLogData.Value(command.LogDataKey).(command.LogData).Username)
		})
	}
}

func TestFailingExecute(t *testing.T) {
	url := testserver.StartSocketHTTPServer(t, requests)

	identityFields := []string{userNameField, keyIDField, krb5PrincipalField}

	testCases := []struct {
		desc              string
		arguments         *commandargs.Shell
		expectedError     string
		expectedLogMsg    string
		expectedLogFields map[string]any
	}{
		{
			desc:          "With missing arguments",
			arguments:     &commandargs.Shell{},
			expectedError: "Failed to get username: who='' is invalid",
		},
		{
			desc:              "When the API returns an error",
			arguments:         &commandargs.Shell{GitlabUsername: brokenUsername},
			expectedError:     forbiddenError,
			expectedLogMsg:    clientErrorMsg,
			expectedLogFields: map[string]any{userNameField: brokenUsername},
		},
		{
			desc:              "When the API returns an error for a key id",
			arguments:         &commandargs.Shell{GitlabKeyID: brokenKeyID},
			expectedError:     forbiddenError,
			expectedLogMsg:    clientErrorMsg,
			expectedLogFields: map[string]any{keyIDField: float64(2)},
		},
		{
			desc:              "When the API returns an error for a Kerberos principal",
			arguments:         &commandargs.Shell{GitlabKrb5Principal: brokenKrb5Principal},
			expectedError:     forbiddenError,
			expectedLogMsg:    clientErrorMsg,
			expectedLogFields: map[string]any{krb5PrincipalField: brokenKrb5Principal},
		},
		{
			desc:              "When the API fails",
			arguments:         &commandargs.Shell{GitlabUsername: unreachableUsername},
			expectedError:     "Failed to get username: Internal API unreachable",
			expectedLogMsg:    apiErrorMsg,
			expectedLogFields: map[string]any{userNameField: unreachableUsername},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			var logBuf bytes.Buffer
			ctx := log.WithLogger(context.Background(), slog.New(slog.NewJSONHandler(&logBuf, nil)))

			buffer := &bytes.Buffer{}
			cmd := &Command{
				Config:     &config.Config{GitlabURL: url},
				Args:       tc.arguments,
				ReadWriter: &readwriter.ReadWriter{Out: buffer},
			}

			_, err := cmd.Execute(ctx)

			require.Empty(t, buffer.String())
			require.EqualError(t, err, tc.expectedError)

			if tc.expectedLogMsg == "" {
				require.Empty(t, logBuf.String())
				return
			}

			entry := findLogEntry(t, logBuf.Bytes(), tc.expectedLogMsg)
			for _, field := range identityFields {
				if value, ok := tc.expectedLogFields[field]; ok {
					require.Equal(t, value, entry[field])
				} else {
					require.NotContains(t, entry, field)
				}
			}
		})
	}
}

func findLogEntry(t *testing.T, logs []byte, msg string) map[string]any {
	t.Helper()

	for _, line := range bytes.Split(bytes.TrimRight(logs, "\n"), []byte("\n")) {
		var entry map[string]any
		if json.Unmarshal(line, &entry) == nil && entry["msg"] == msg {
			return entry
		}
	}

	require.Failf(t, "log entry not found", "no log line with msg %q", msg)
	return nil
}
