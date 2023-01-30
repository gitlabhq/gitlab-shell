package twofactorrecover

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/twofactorrecover"
)

var requests []testserver.TestRequestHandler

func setup(t *testing.T) {
	requests = []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/two_factor_recovery_codes",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				b, err := io.ReadAll(r.Body)
				defer r.Body.Close()

				require.NoError(t, err)

				var requestBody *twofactorrecover.RequestBody
				json.Unmarshal(b, &requestBody)

				switch requestBody.KeyId {
				case "1":
					body := map[string]interface{}{
						"success":        true,
						"recovery_codes": [2]string{"recovery", "codes"},
					}
					json.NewEncoder(w).Encode(body)
				case "forbidden":
					body := map[string]interface{}{
						"success": false,
						"message": "Forbidden!",
					}
					json.NewEncoder(w).Encode(body)
				case "broken":
					w.WriteHeader(http.StatusInternalServerError)
				}
			},
		},
	}
}

const (
	question = "Are you sure you want to generate new two-factor recovery codes?\n" +
		"Any existing recovery codes you saved will be invalidated. (yes/no)\n\n"
	errorHeader = "An error occurred while trying to generate new recovery codes.\n"
)

func TestExecute(t *testing.T) {
	setup(t)

	url := testserver.StartSocketHttpServer(t, requests)

	testCases := []struct {
		desc           string
		arguments      *commandargs.Shell
		answer         string
		expectedOutput string
	}{
		{
			desc:      "With a known key id",
			arguments: &commandargs.Shell{GitlabKeyId: "1"},
			answer:    "yes\n",
			expectedOutput: question +
				"Your two-factor authentication recovery codes are:\n\nrecovery\ncodes\n\n" +
				"During sign in, use one of the codes above when prompted for\n" +
				"your two-factor code. Then, visit your Profile Settings and add\n" +
				"a new device so you do not lose access to your account again.\n",
		},
		{
			desc:           "With bad response",
			arguments:      &commandargs.Shell{GitlabKeyId: "-1"},
			answer:         "yes\n",
			expectedOutput: question + errorHeader + "Parsing failed\n",
		},
		{
			desc:           "With API returns an error",
			arguments:      &commandargs.Shell{GitlabKeyId: "forbidden"},
			answer:         "yes\n",
			expectedOutput: question + errorHeader + "Forbidden!\n",
		},
		{
			desc:           "With API fails",
			arguments:      &commandargs.Shell{GitlabKeyId: "broken"},
			answer:         "yes\n",
			expectedOutput: question + errorHeader + "Internal API unreachable\n",
		},
		{
			desc:           "With missing arguments",
			arguments:      &commandargs.Shell{},
			answer:         "yes\n",
			expectedOutput: question + errorHeader + "who='' is invalid\n",
		},
		{
			desc:      "With negative answer",
			arguments: &commandargs.Shell{},
			answer:    "no\n",
			expectedOutput: question +
				"New recovery codes have *not* been generated. Existing codes will remain valid.\n",
		},
		{
			desc:      "With some other answer",
			arguments: &commandargs.Shell{},
			answer:    strings.Repeat("yes", 1024),
			expectedOutput: question +
				"New recovery codes have *not* been generated. Existing codes will remain valid.\n",
		},
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
