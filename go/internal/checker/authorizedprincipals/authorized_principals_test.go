package authorizedprincipals

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/config"
)

func TestExecute(t *testing.T) {
	testCases := []struct {
		desc           string
		arguments      []string
		expectedOutput string
	}{
		{
			desc:           "With single principal",
			arguments:      []string{"key", "principal"},
			expectedOutput: "command=\"/tmp/bin/gitlab-shell username-key\",no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty principal\n",
		},
		{
			desc:           "With mulitple principals",
			arguments:      []string{"key", "principal-1", "principal-2"},
			expectedOutput: "command=\"/tmp/bin/gitlab-shell username-key\",no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty principal-1\ncommand=\"/tmp/bin/gitlab-shell username-key\",no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty principal-2\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			buffer := &bytes.Buffer{}
			cmd := &Checker{
				Config:     &config.Config{RootDir: "/tmp"},
				Args:       tc.arguments,
				ReadWriter: &readwriter.ReadWriter{Out: buffer},
			}

			err := cmd.Execute()

			require.NoError(t, err)
			require.Equal(t, tc.expectedOutput, buffer.String())
		})
	}
}

func TestFailingExecute(t *testing.T) {
	testCases := []struct {
		desc          string
		arguments     []string
		expectedError string
	}{
		{
			desc:          "With wrong number of arguments",
			arguments:     []string{"key"},
			expectedError: "# Wrong number of arguments. 1. Usage\n#\tgitlab-shell-authorized-principals-check <key-id> <principal1> [<principal2>...]",
		},
		{
			desc:          "With missing key_id",
			arguments:     []string{"", "principal"},
			expectedError: "# No key_id provided",
		},
		{
			desc:          "With blank principal",
			arguments:     []string{"key", "principal", ""},
			expectedError: "# An invalid principal was provided",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			buffer := &bytes.Buffer{}
			cmd := &Checker{
				Config:     &config.Config{},
				Args:       tc.arguments,
				ReadWriter: &readwriter.ReadWriter{Out: buffer},
			}

			err := cmd.Execute()

			require.Empty(t, buffer.String())
			require.EqualError(t, err, tc.expectedError)
		})
	}
}
