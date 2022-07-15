package twofactorverify

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet/twofactorverify"
)

func setupManual(t *testing.T) []testserver.TestRequestHandler {
	requests := []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/two_factor_manual_otp_check",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				b, err := io.ReadAll(r.Body)
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
		{
			Path: "/api/v4/internal/two_factor_push_otp_check",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				b, err := io.ReadAll(r.Body)
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
				default:
					body = map[string]interface{}{
						"success": true,
						"message": "default message",
					}
					json.NewEncoder(w).Encode(body)
				}
			},
		},
	}

	return requests
}

const (
	manualQuestion    = "OTP: \n"
	manualErrorHeader = "OTP validation failed.\n"
)

func TestExecuteManual(t *testing.T) {
	requests := setupManual(t)

	url := testserver.StartSocketHttpServer(t, requests)

	testCases := []struct {
		desc           string
		arguments      *commandargs.Shell
		answer         string
		expectedOutput string
	}{
		{
			desc:           "With a known key id",
			arguments:      &commandargs.Shell{GitlabKeyId: "1"},
			answer:         "123456\n",
			expectedOutput: manualQuestion + "OTP validation successful. Git operations are now allowed.\n",
		},
		//{
		//	desc:           "With bad response",
		//	arguments:      &commandargs.Shell{GitlabKeyId: "-1"},
		//	answer:         "123456\n",
		//	expectedOutput: manualQuestion + manualErrorHeader + "Parsing failed\n",
		//},
		//{
		//	desc:           "With API returns an error",
		//	arguments:      &commandargs.Shell{GitlabKeyId: "error"},
		//	answer:         "yes\n",
		//	expectedOutput: manualQuestion + manualErrorHeader + "error message\n",
		//},
		//{
		//	desc:           "With API fails",
		//	arguments:      &commandargs.Shell{GitlabKeyId: "broken"},
		//	answer:         "yes\n",
		//	expectedOutput: manualQuestion + manualErrorHeader + "Internal API error (500)\n",
		//},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			output := &bytes.Buffer{}

			input := bytes.NewBufferString(tc.answer)

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
