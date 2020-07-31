package receivepack

import (
	"bytes"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/testhelper"
	"gitlab.com/gitlab-org/gitlab-shell/internal/testhelper/requesthandlers"
)

func TestReceivePack(t *testing.T) {
	gitalyAddress, _, cleanup := testserver.StartGitalyServer(t)
	defer cleanup()

	requests := requesthandlers.BuildAllowedWithGitalyHandlers(t, gitalyAddress)
	url, cleanup := testserver.StartHttpServer(t, requests)
	defer cleanup()

	envCleanup, err := testhelper.Setenv("SSH_CONNECTION", "127.0.0.1 0")
	require.NoError(t, err)
	defer envCleanup()

	output := &bytes.Buffer{}
	input := &bytes.Buffer{}

	userId := "1"
	repo := "group/repo"

	cmd := &Command{
		Config:     &config.Config{GitlabUrl: url},
		Args:       &commandargs.Shell{GitlabKeyId: userId, CommandType: commandargs.ReceivePack, SshArgs: []string{"git-receive-pack", repo}},
		ReadWriter: &readwriter.ReadWriter{ErrOut: output, Out: output, In: input},
	}

	hook := testhelper.SetupLogger()

	err = cmd.Execute()
	require.NoError(t, err)

	require.Equal(t, "ReceivePack: "+userId+" "+repo, output.String())

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
