package twofactorverify

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet/twofactorverify"
)

func setup(t *testing.T) []testserver.TestRequestHandler {
	requests := []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/two_factor_otp_check",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				b, err := ioutil.ReadAll(r.Body)
				defer r.Body.Close()

				require.NoError(t, err)

				var requestBody *twofactorverify.RequestBody
				require.NoError(t, json.Unmarshal(b, &requestBody))

				var body map[string]interface{}
				switch requestBody.KeyId {
				case "1":
					body = map[string]interface{}{
						"success": true,
					}
					json.NewEncoder(w).Encode(body)
				case "error":
					body = map[string]interface{}{
						"success": false,
						"message": "error message",
					}
					require.NoError(t, json.NewEncoder(w).Encode(body))
				case "broken":
					w.WriteHeader(http.StatusInternalServerError)
				}
			},
		},
	}

	return requests
}

const (
	question    = "OTP: \n"
	errorHeader = "OTP validation failed.\n"
)

func TestExecute(t *testing.T) {
	requests := setup(t)

	url, cleanup := testserver.StartSocketHttpServer(t, requests)
	defer cleanup()

	testCases := []struct {
		desc           string
		arguments      *commandargs.Shell
		answer         string
		expectedOutput string
	}{
		{
			desc:           "When push is provided",
			arguments:      &commandargs.Shell{GitlabKeyId: "1"},
			answer:         "",
			expectedOutput: question + "Push OTP validation successful. Git operations are now allowed.\n",
		},
		{
			desc:           "With a known key id",
			arguments:      &commandargs.Shell{GitlabKeyId: "1"},
			answer:         "123456\n",
			expectedOutput: question + "OTP validation successful. Git operations are now allowed.\n",
		},
		{
			desc:           "With bad response",
			arguments:      &commandargs.Shell{GitlabKeyId: "-1"},
			answer:         "123456\n",
			expectedOutput: question + errorHeader + "Parsing failed\n",
		},
		{
			desc:           "With API returns an error",
			arguments:      &commandargs.Shell{GitlabKeyId: "error"},
			answer:         "yes\n",
			expectedOutput: question + errorHeader + "error message\n",
		},
		{
			desc:           "With API fails",
			arguments:      &commandargs.Shell{GitlabKeyId: "broken"},
			answer:         "yes\n",
			expectedOutput: question + errorHeader + "Internal API error (500)\n",
		},
		{
			desc:           "With missing arguments",
			arguments:      &commandargs.Shell{},
			answer:         "yes\n",
			expectedOutput: question + errorHeader + "who='' is invalid\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			output := &bytes.Buffer{}

			input := bytes.NewBufferString(tc.answer)
			if tc.answer == "" { // pushauth test must not provide input otherwise it gets treated as a manual OTP
				input.Reset()
			}

			cmd := &Command{
				Config:     &config.Config{GitlabUrl: url},
				Args:       tc.arguments,
				ReadWriter: &readwriter.ReadWriter{Out: output, In: input},
			}

			err := cmd.Execute(context.Background())

			require.NoError(t, err)
			require.Equal(t, tc.expectedOutput, output.String())
		})
	}
}
