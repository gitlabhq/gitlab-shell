package receivepack

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/testhelper/requesthandlers"
)

func TestForbiddenAccess(t *testing.T) {
	requests := requesthandlers.BuildDisallowedByApiHandlers(t)
	cmd, _ := setup(t, "disallowed", requests)

	err := cmd.Execute(context.Background())
	require.Equal(t, "Disallowed by API call", err.Error())
}

func TestCustomReceivePack(t *testing.T) {
	cmd, output := setup(t, "1", requesthandlers.BuildAllowedWithCustomActionsHandlers(t))

	require.NoError(t, cmd.Execute(context.Background()))
	require.Equal(t, "customoutput", output.String())
}

func setup(t *testing.T, keyId string, requests []testserver.TestRequestHandler) (*Command, *bytes.Buffer) {
	url := testserver.StartSocketHttpServer(t, requests)

	output := &bytes.Buffer{}
	input := bytes.NewBufferString("input")

	cmd := &Command{
		Config:     &config.Config{GitlabUrl: url},
		Args:       &commandargs.Shell{GitlabKeyId: keyId, SshArgs: []string{"git-receive-pack", "group/repo"}},
		ReadWriter: &readwriter.ReadWriter{ErrOut: output, Out: output, In: input},
	}

	return cmd, output
}
