package receivepack

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/internal/testhelper/requesthandlers"
)

func TestReceivePack(t *testing.T) {
	gitalyAddress, _, cleanup := testserver.StartGitalyServer(t)
	defer cleanup()

	requests := requesthandlers.BuildAllowedWithGitalyHandlers(t, gitalyAddress)
	url, cleanup := testserver.StartHttpServer(t, requests)
	defer cleanup()

	output := &bytes.Buffer{}
	input := &bytes.Buffer{}

	userId := "1"
	repo := "group/repo"

	cmd := &Command{
		Config:     &config.Config{GitlabUrl: url},
		Args:       &commandargs.Shell{GitlabKeyId: userId, CommandType: commandargs.ReceivePack, SshArgs: []string{"git-receive-pack", repo}},
		ReadWriter: &readwriter.ReadWriter{ErrOut: output, Out: output, In: input},
	}

	err := cmd.Execute()
	require.NoError(t, err)

	require.Equal(t, "ReceivePack: "+userId+" "+repo, output.String())
}
