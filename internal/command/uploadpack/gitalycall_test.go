package uploadpack

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/labkit/correlation"

	"gitlab.com/gitlab-org/gitlab-shell/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/testhelper"
	"gitlab.com/gitlab-org/gitlab-shell/internal/testhelper/requesthandlers"
)

func TestUploadPack(t *testing.T) {
	gitalyAddress, testServer := testserver.StartGitalyServer(t)

	requests := requesthandlers.BuildAllowedWithGitalyHandlers(t, gitalyAddress)
	url := testserver.StartHttpServer(t, requests)

	output := &bytes.Buffer{}
	input := &bytes.Buffer{}

	userId := "1"
	repo := "group/repo"

	cmd := &Command{
		Config:     &config.Config{GitlabUrl: url},
		Args:       &commandargs.Shell{GitlabKeyId: userId, CommandType: commandargs.UploadPack, SshArgs: []string{"git-upload-pack", repo}},
		ReadWriter: &readwriter.ReadWriter{ErrOut: output, Out: output, In: input},
	}

	hook := testhelper.SetupLogger()
	ctx := correlation.ContextWithCorrelation(context.Background(), "a-correlation-id")
	ctx = correlation.ContextWithClientName(ctx, "gitlab-shell-tests")

	err := cmd.Execute(ctx)
	require.NoError(t, err)

	require.Equal(t, "UploadPack: "+repo, output.String())
	require.Eventually(t, func() bool {
		entries := hook.AllEntries()

		require.Equal(t, 2, len(entries))
		require.Contains(t, entries[1].Message, "executing git command")
		require.Contains(t, entries[1].Message, "command=git-upload-pack")
		require.Contains(t, entries[1].Message, "gl_key_type=key")
		require.Contains(t, entries[1].Message, "gl_key_id=123")
		require.Contains(t, entries[1].Message, "correlation_id=a-correlation-id")
		return true
	}, time.Second, time.Millisecond)

	for k, v := range map[string]string{
		"gitaly-feature-cache_invalidator":        "true",
		"gitaly-feature-inforef_uploadpack_cache": "false",
	} {
		actual := testServer.ReceivedMD[k]
		require.Len(t, actual, 1)
		require.Equal(t, v, actual[0])
	}
	require.Empty(t, testServer.ReceivedMD["some-other-ff"])
}
