package uploadarchive

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

func TestUploadArchive(t *testing.T) {
	for _, network := range []string{"unix", "tcp", "dns"} {
		t.Run(fmt.Sprintf("via %s network", network), func(t *testing.T) {
			gitalyAddress, testServer := testserver.StartGitalyServer(t, network)
			t.Logf("Server address: %s", gitalyAddress)

			requests := requesthandlers.BuildAllowedWithGitalyHandlers(t, gitalyAddress)
			url := testserver.StartHTTPServer(t, requests)

			output := &bytes.Buffer{}
			input := &bytes.Buffer{}

			userID := "1"
			repo := "group/repo"

			env := sshenv.Env{
				IsSSHConnection: true,
				OriginalCommand: "git-upload-archive " + repo,
				RemoteAddr:      "127.0.0.1",
			}

			args := &commandargs.Shell{
				GitlabKeyID: userID,
				CommandType: commandargs.UploadArchive,
				SSHArgs:     []string{"git-upload-archive", repo},
				Env:         env,
			}

			cfg := &config.Config{GitlabUrl: url}
			cfg.GitalyClient.InitSidechannelRegistry(context.Background())
			cmd := &Command{
				Config:     cfg,
				Args:       args,
				ReadWriter: &readwriter.ReadWriter{ErrOut: output, Out: output, In: input},
			}

			correlationID := correlation.SafeRandomID()
			ctx := correlation.ContextWithCorrelation(context.Background(), correlationID)
			ctx = correlation.ContextWithClientName(ctx, "gitlab-shell-tests")

			_, err := cmd.Execute(ctx)
			require.NoError(t, err)

			require.Equal(t, "UploadArchive: "+repo, output.String())

			for k, v := range map[string]string{
				"gitaly-feature-cache_invalidator":        "true",
				"gitaly-feature-inforef_uploadpack_cache": "false",
				"x-gitlab-client-name":                    "gitlab-shell-tests-git-upload-archive",
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
			require.Equal(t, testServer.ReceivedMD["x-gitlab-correlation-id"][0], correlationID)
		})
	}
}
