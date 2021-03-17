package receivepack

import (
	"bytes"
	"context"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/sshenv"
	"gitlab.com/gitlab-org/gitlab-shell/internal/testhelper"
	"gitlab.com/gitlab-org/gitlab-shell/internal/testhelper/requesthandlers"
)

func TestReceivePack(t *testing.T) {
	gitalyAddress, _ := testserver.StartGitalyServer(t)

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
			OriginalCommand: "git-receive-pack group/repo",
			RemoteAddr:      "127.0.0.1",
		}
		args := &commandargs.Shell{CommandType: commandargs.ReceivePack, SshArgs: []string{"git-receive-pack", repo}, Env: env}

		if tc.username != "" {
			args.GitlabUsername = tc.username
		} else {
			args.GitlabKeyId = tc.keyId
		}

		cmd := &Command{
			Config:     &config.Config{GitlabUrl: url},
			Args:       args,
			ReadWriter: &readwriter.ReadWriter{ErrOut: output, Out: output, In: input},
		}

		hook := testhelper.SetupLogger()

		err := cmd.Execute(context.Background())
		require.NoError(t, err)

		if tc.username != "" {
			require.Equal(t, "ReceivePack: 1 "+repo, output.String())
		} else {
			require.Equal(t, "ReceivePack: key-123 "+repo, output.String())
		}

		require.True(t, testhelper.WaitForLogEvent(hook))
		entries := hook.AllEntries()
		require.Equal(t, 2, len(entries))
		require.Equal(t, logrus.InfoLevel, entries[1].Level)
		require.Contains(t, entries[1].Message, "executing git command")
		require.Contains(t, entries[1].Message, "command=git-receive-pack")
		require.Contains(t, entries[1].Message, "remote_ip=127.0.0.1")
		require.Contains(t, entries[1].Message, "gl_key_type=key")
		require.Contains(t, entries[1].Message, "gl_key_id=123")
		require.Contains(t, entries[1].Message, "correlation_id=")
	}
}
