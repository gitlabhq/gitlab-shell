package receivepack

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/testhelper/gitalytest"
)

func TestReceivePack(t *testing.T) {
	const repo = "group/repo"

	testCases := []struct{ username, keyID string }{
		{username: "john.doe"},
		{keyID: "123"},
	}

	gitalytest.RunAllNetworks(t, func(t *testing.T, servers gitalytest.Servers) {
		for _, tc := range testCases {
			setup := gitalytest.NewCommandSetup(gitalytest.CommandOptions{
				GitlabURL:      servers.GitlabURL,
				CommandType:    commandargs.ReceivePack,
				Repository:     repo,
				RemoteAddr:     "127.0.0.1",
				GitlabUsername: tc.username,
				GitlabKeyID:    tc.keyID,
				CorrelationID:  "a-correlation-id",
				ClientName:     "gitlab-shell-tests",
			})
			cmd := &Command{Config: setup.Config, Args: setup.Args, ReadWriter: setup.ReadWriter}
			_, err := cmd.Execute(setup.Context)
			require.NoError(t, err)

			if tc.username != "" {
				require.Equal(t, "ReceivePack: user-1 "+repo, setup.Output.String())
			} else {
				require.Equal(t, "ReceivePack: key-123 "+repo, setup.Output.String())
			}
			gitalytest.RequireMetadata(t, servers.Gitaly, gitalytest.MetadataExpectations{
				ClientNameSuffix: "git-receive-pack",
				CorrelationID:    "a-correlation-id",
				RemoteAddr:       "127.0.0.1",
			})
		}
	})
}
