package personalaccesstoken

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/personalaccesstoken"
)

var requests []testserver.TestRequestHandler

func setup(t *testing.T) {
	requests = []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/personal_access_token",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				b, err := io.ReadAll(r.Body)
				defer r.Body.Close()

				require.NoError(t, err)

				var requestBody *personalaccesstoken.RequestBody
				json.Unmarshal(b, &requestBody)

				switch requestBody.KeyId {
				case "forbidden":
					body := map[string]interface{}{
						"success": false,
						"message": "Forbidden!",
					}
					json.NewEncoder(w).Encode(body)
				case "broken":
					w.WriteHeader(http.StatusInternalServerError)
				case "badresponse":
				default:
					var expiresAt interface{}
					if requestBody.ExpiresAt == "" {
						expiresAt = nil
					} else {
						expiresAt = "9001-11-17"
					}
					body := map[string]interface{}{
						"success":    true,
						"token":      "YXuxvUgCEmeePY3G1YAa",
						"scopes":     requestBody.Scopes,
						"expires_at": expiresAt,
					}
					json.NewEncoder(w).Encode(body)
				}
			},
		},
	}
}

const (
	cmdname = "personal_access_token"
)

func TestExecute(t *testing.T) {
	setup(t)

	url := testserver.StartSocketHttpServer(t, requests)

	testCases := []struct {
		desc           string
		arguments      *commandargs.Shell
		expectedOutput string
		expectedError  string
	}{
		{
			desc:          "Without any arguments",
			arguments:     &commandargs.Shell{},
			expectedError: usageText,
		},
		{
			desc: "With too few arguments",
			arguments: &commandargs.Shell{
				SshArgs: []string{cmdname, "newtoken"},
			},
			expectedError: usageText,
		},
		{
			desc: "With too many arguments",
			arguments: &commandargs.Shell{
				SshArgs: []string{cmdname, "newtoken", "api", "bad_ttl", "toomany"},
			},
			expectedError: usageText,
		},
		{
			desc: "With a bad ttl_days argument",
			arguments: &commandargs.Shell{
				SshArgs: []string{cmdname, "newtoken", "api", "bad_ttl"},
			},
			expectedError: "Invalid value for days_ttl: 'bad_ttl'",
		},
		{
			desc: "Without a ttl argument",
			arguments: &commandargs.Shell{
				GitlabKeyId: "default",
				SshArgs:     []string{cmdname, "newtoken", "read_api,read_repository"},
			},
			expectedOutput: "Token:   YXuxvUgCEmeePY3G1YAa\n" +
				"Scopes:  read_api,read_repository\n" +
				"Expires: never\n",
		},
		{
			desc: "With a ttl argument",
			arguments: &commandargs.Shell{
				GitlabKeyId: "default",
				SshArgs:     []string{cmdname, "newtoken", "api", "30"},
			},
			expectedOutput: "Token:   YXuxvUgCEmeePY3G1YAa\n" +
				"Scopes:  api\n" +
				"Expires: 9001-11-17\n",
		},
		{
			desc: "With bad response",
			arguments: &commandargs.Shell{
				GitlabKeyId: "badresponse",
				SshArgs:     []string{cmdname, "newtoken", "read_api,read_repository"},
			},
			expectedError: "Parsing failed",
		},
		{
			desc: "when API returns an error",
			arguments: &commandargs.Shell{
				GitlabKeyId: "forbidden",
				SshArgs:     []string{cmdname, "newtoken", "read_api,read_repository"},
			},
			expectedError: "Forbidden!",
		},
		{
			desc: "When API fails",
			arguments: &commandargs.Shell{
				GitlabKeyId: "broken",
				SshArgs:     []string{cmdname, "newtoken", "read_api,read_repository"},
			},
			expectedError: "Internal API unreachable",
		},
		{
			desc: "Without KeyID or User",
			arguments: &commandargs.Shell{
				SshArgs: []string{cmdname, "newtoken", "read_api,read_repository"},
			},
			expectedError: "who='' is invalid",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			output := &bytes.Buffer{}
			input := bytes.NewBufferString("")

			cmd := &Command{
				Config:     &config.Config{GitlabUrl: url},
				Args:       tc.arguments,
				ReadWriter: &readwriter.ReadWriter{Out: output, In: input},
			}

			err := cmd.Execute(context.Background())

			if tc.expectedError == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tc.expectedError)
			}

			if tc.expectedOutput != "" {
				require.Equal(t, tc.expectedOutput, output.String())
			}
		})
	}
}
