package personalaccesstoken

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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

				assert.NoError(t, err)

				var requestBody *personalaccesstoken.RequestBody
				json.Unmarshal(b, &requestBody)

				switch requestBody.KeyID {
				case "forbidden":
					body := map[string]interface{}{
						"success": false,
						"message": "Forbidden!",
					}
					json.NewEncoder(w).Encode(body)
				case "broken":
					w.WriteHeader(http.StatusInternalServerError)
				case "invalidscope":
					message := "Invalid scope: '" + strings.Join(requestBody.Scopes, ",") + "'."
					message += " Valid scopes are: [\"api\", \"create_runner\", \"k8s_proxy\", \"read_api\", \"read_registry\", \"read_repository\", \"read_user\", \"write_registry\", \"write_repository\"]"
					body := map[string]interface{}{
						"success": false,
						"message": message,
					}
					json.NewEncoder(w).Encode(body)
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

	url := testserver.StartSocketHTTPServer(t, requests)

	testCases := []struct {
		desc           string
		PATConfig      config.PATConfig
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
			expectedError: "invalid value for days_ttl: 'bad_ttl'",
		},
		{
			desc: "Without a ttl argument",
			arguments: &commandargs.Shell{
				GitlabKeyId: "default",
				SshArgs:     []string{cmdname, "newtoken", "read_api,read_repository"},
			},
			expectedOutput: "Token:   YXuxvUgCEmeePY3G1YAa\n" +
				"Scopes:  read_api,read_repository\n" +
				"Expires: 9001-11-17\n",
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
			expectedError: "parsing failed",
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
		{
			desc:      "With restricted scopes",
			PATConfig: config.PATConfig{AllowedScopes: []string{"read_api"}},
			arguments: &commandargs.Shell{
				GitlabKeyId: "default",
				SshArgs:     []string{cmdname, "newtoken", "read_api,read_repository"},
			},
			expectedOutput: "Token:   YXuxvUgCEmeePY3G1YAa\n" +
				"Scopes:  read_api\n" +
				"Expires: 9001-11-17\n",
		},
		{
			desc:      "With unknown configured scopes",
			PATConfig: config.PATConfig{AllowedScopes: []string{"unknown_repository"}},
			arguments: &commandargs.Shell{
				GitlabKeyId: "default",
				SshArgs:     []string{cmdname, "newtoken", "read_api,read_repository"},
			},
			expectedOutput: "Token:   YXuxvUgCEmeePY3G1YAa\n" +
				"Scopes:  \n" +
				"Expires: 9001-11-17\n",
		},
		{
			desc:      "With unknown requested scopes",
			PATConfig: config.PATConfig{AllowedScopes: []string{"read_api", "read_repository"}},
			arguments: &commandargs.Shell{
				GitlabKeyId: "default",
				SshArgs:     []string{cmdname, "newtoken", "read_api,unknown_repository"},
			},
			expectedOutput: "Token:   YXuxvUgCEmeePY3G1YAa\n" +
				"Scopes:  read_api\n" +
				"Expires: 9001-11-17\n",
		},
		{
			desc:      "With matching unknown requested scopes",
			PATConfig: config.PATConfig{AllowedScopes: []string{"read_api", "unknown_repository"}},
			arguments: &commandargs.Shell{
				GitlabKeyId: "invalidscope",
				SshArgs:     []string{cmdname, "newtoken", "unknown_repository"},
			},
			expectedError: "Invalid scope: 'unknown_repository'. Valid scopes are: [\"api\", \"create_runner\", \"k8s_proxy\", \"read_api\", \"read_registry\", \"read_repository\", \"read_user\", \"write_registry\", \"write_repository\"]",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			output := &bytes.Buffer{}
			input := bytes.NewBufferString("")

			cmd := &Command{
				Config:     &config.Config{GitlabUrl: url, PATConfig: tc.PATConfig},
				Args:       tc.arguments,
				ReadWriter: &readwriter.ReadWriter{Out: output, In: input},
			}

			_, err := cmd.Execute(context.Background())

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
