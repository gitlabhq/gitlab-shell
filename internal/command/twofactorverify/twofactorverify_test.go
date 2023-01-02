package twofactorverify

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
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/twofactorverify"
)

type blockingReader struct{}

func (*blockingReader) Read([]byte) (int, error) {
	waitInfinitely := make(chan struct{})
	<-waitInfinitely

	return 0, nil
}

func setup(t *testing.T) []testserver.TestRequestHandler {
	waitInfinitely := make(chan struct{})
	requests := []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/two_factor_manual_otp_check",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				b, err := io.ReadAll(r.Body)
				defer r.Body.Close()

				require.NoError(t, err)

				var requestBody *twofactorverify.RequestBody
				require.NoError(t, json.Unmarshal(b, &requestBody))

				switch requestBody.KeyId {
				case "verify_via_otp", "verify_via_otp_with_push_error":
					body := map[string]interface{}{
						"success": true,
					}
					json.NewEncoder(w).Encode(body)
				case "wait_infinitely":
					<-waitInfinitely
				case "error":
					body := map[string]interface{}{
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

				switch requestBody.KeyId {
				case "verify_via_push":
					body := map[string]interface{}{
						"success": true,
					}
					json.NewEncoder(w).Encode(body)
				case "verify_via_otp_with_push_error":
					w.WriteHeader(http.StatusInternalServerError)
				default:
					<-waitInfinitely
				}
			},
		},
	}

	return requests
}

const errorHeader = "OTP validation failed: "

func TestExecute(t *testing.T) {
	requests := setup(t)

	url := testserver.StartSocketHttpServer(t, requests)

	testCases := []struct {
		desc           string
		arguments      *commandargs.Shell
		input          io.Reader
		expectedOutput string
	}{
		{
			desc:           "Verify via OTP",
			arguments:      &commandargs.Shell{GitlabKeyId: "verify_via_otp"},
			expectedOutput: "OTP validation successful. Git operations are now allowed.\n",
		},
		{
			desc:           "Verify via OTP",
			arguments:      &commandargs.Shell{GitlabKeyId: "verify_via_otp_with_push_error"},
			expectedOutput: "OTP validation successful. Git operations are now allowed.\n",
		},
		{
			desc:           "Verify via push authentication",
			arguments:      &commandargs.Shell{GitlabKeyId: "verify_via_push"},
			input:          &blockingReader{},
			expectedOutput: "OTP has been validated by Push Authentication. Git operations are now allowed.\n",
		},
		{
			desc:           "With an empty OTP",
			arguments:      &commandargs.Shell{GitlabKeyId: "verify_via_otp"},
			input:          bytes.NewBufferString("\n"),
			expectedOutput: errorHeader + "OTP cannot be blank.\n",
		},
		{
			desc:           "With bad response",
			arguments:      &commandargs.Shell{GitlabKeyId: "-1"},
			expectedOutput: errorHeader + "Parsing failed\n",
		},
		{
			desc:           "With API returns an error",
			arguments:      &commandargs.Shell{GitlabKeyId: "error"},
			expectedOutput: errorHeader + "error message\n",
		},
		{
			desc:           "With API fails",
			arguments:      &commandargs.Shell{GitlabKeyId: "broken"},
			expectedOutput: errorHeader + "Internal API unreachable\n",
		},
		{
			desc:           "With missing arguments",
			arguments:      &commandargs.Shell{},
			expectedOutput: errorHeader + "who='' is invalid\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			output := &bytes.Buffer{}

			input := tc.input
			if input == nil {
				input = bytes.NewBufferString("123456\n")
			}

			cmd := &Command{
				Config:     &config.Config{GitlabUrl: url},
				Args:       tc.arguments,
				ReadWriter: &readwriter.ReadWriter{Out: output, In: input},
			}

			err := cmd.Execute(context.Background())

			require.NoError(t, err)
			require.Equal(t, prompt+"\n"+tc.expectedOutput, output.String())
		})
	}
}

func TestCanceledContext(t *testing.T) {
	requests := setup(t)

	output := &bytes.Buffer{}

	url := testserver.StartSocketHttpServer(t, requests)
	cmd := &Command{
		Config:     &config.Config{GitlabUrl: url},
		Args:       &commandargs.Shell{GitlabKeyId: "wait_infinitely"},
		ReadWriter: &readwriter.ReadWriter{Out: output, In: &blockingReader{}},
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error)
	go func() { errCh <- cmd.Execute(ctx) }()
	cancel()

	require.NoError(t, <-errCh)
	require.Equal(t, prompt+"\n"+errorHeader+"context canceled\n", output.String())
}
