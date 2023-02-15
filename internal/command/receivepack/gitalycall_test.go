package receivepack

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/labkit/correlation"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/sshenv"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/testhelper/requesthandlers"
)

func TestReceivePack(t *testing.T) {
	for _, network := range []string{"unix", "tcp", "dns"} {
		t.Run(fmt.Sprintf("via %s network", network), func(t *testing.T) {
			gitalyAddress, testServer := testserver.StartGitalyServer(t, network)
			t.Log(fmt.Sprintf("Server address: %s", gitalyAddress))

			requests := requesthandlers.BuildAllowedWithGitalyHandlers(t, gitalyAddress)
			url := testserver.StartHttpServer(t, requests)

			testCases := []struct {
				username string
				keyId    string
			}{
				{
					username: "john.doe",
				},
				{
					keyId: "123",
				},
			}

			for _, tc := range testCases {
				output := &bytes.Buffer{}
				input := &bytes.Buffer{}
				repo := "group/repo"

				env := sshenv.Env{
					IsSSHConnection: true,
					OriginalCommand: "git-receive-pack " + repo,
					RemoteAddr:      "127.0.0.1",
				}

				args := &commandargs.Shell{
					CommandType: commandargs.ReceivePack,
					SshArgs:     []string{"git-receive-pack", repo},
					Env:         env,
				}

				if tc.username != "" {
					args.GitlabUsername = tc.username
				} else {
					args.GitlabKeyId = tc.keyId
				}

				cfg := &config.Config{GitlabUrl: url}
				cfg.GitalyClient.InitSidechannelRegistry(context.Background())
				cmd := &Command{
					Config:     cfg,
					Args:       args,
					ReadWriter: &readwriter.ReadWriter{ErrOut: output, Out: output, In: input},
				}

				ctx := correlation.ContextWithCorrelation(context.Background(), "a-correlation-id")
				ctx = correlation.ContextWithClientName(ctx, "gitlab-shell-tests")

				err := cmd.Execute(ctx)
				require.NoError(t, err)

				if tc.username != "" {
					require.Equal(t, "ReceivePack: 1 "+repo, output.String())
				} else {
					require.Equal(t, "ReceivePack: key-123 "+repo, output.String())
				}

				for k, v := range map[string]string{
					"gitaly-feature-cache_invalidator":        "true",
					"gitaly-feature-inforef_uploadpack_cache": "false",
					"x-gitlab-client-name":                    "gitlab-shell-tests-git-receive-pack",
					"key_id":                                  "123",
					"user_id":                                 "1",
					"remote_ip":                               "127.0.0.1",
					"key_type":                                "key",
				} {
					actual := testServer.ReceivedMD[k]
					require.Len(t, actual, 1)
					require.Equal(t, v, actual[0])
				}
				require.Empty(t, testServer.ReceivedMD["some-other-ff"])
				require.Equal(t, testServer.ReceivedMD["x-gitlab-correlation-id"][0], "a-correlation-id")
			}
		})
	}
}
