package authorizedprincipals

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
)

func TestExecute(t *testing.T) {
	defaultConfig := &config.Config{RootDir: "/tmp"}

	testCases := []struct {
		desc           string
		arguments      *commandargs.AuthorizedPrincipals
		expectedOutput string
	}{
		{
			desc:           "With single principal",
			arguments:      &commandargs.AuthorizedPrincipals{KeyId: "key", Principals: []string{"principal"}},
			expectedOutput: "command=\"/tmp/bin/gitlab-shell username-key\",no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty principal\n",
		},
		{
			desc:           "With multiple principals",
			arguments:      &commandargs.AuthorizedPrincipals{KeyId: "key", Principals: []string{"principal-1", "principal-2"}},
			expectedOutput: "command=\"/tmp/bin/gitlab-shell username-key\",no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty principal-1\ncommand=\"/tmp/bin/gitlab-shell username-key\",no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty principal-2\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			buffer := &bytes.Buffer{}

			cmd := &Command{
				Config:     defaultConfig,
				Args:       tc.arguments,
				ReadWriter: &readwriter.ReadWriter{Out: buffer},
			}

			err := cmd.Execute(context.Background())

			require.NoError(t, err)
			require.Equal(t, tc.expectedOutput, buffer.String())
		})
	}
}
