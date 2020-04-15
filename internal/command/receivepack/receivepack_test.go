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

func TestForbiddenAccess(t *testing.T) {
	requests := requesthandlers.BuildDisallowedByApiHandlers(t)
	cmd, _, cleanup := setup(t, "disallowed", requests)
	defer cleanup()

	err := cmd.Execute()
	require.Equal(t, "Disallowed by API call", err.Error())
}

func TestCustomReceivePack(t *testing.T) {
	cmd, output, cleanup := setup(t, "1", requesthandlers.BuildAllowedWithCustomActionsHandlers(t))
	defer cleanup()

	require.NoError(t, cmd.Execute())
	require.Equal(t, "customoutput", output.String())
}

func setup(t *testing.T, keyId string, requests []testserver.TestRequestHandler) (*Command, *bytes.Buffer, func()) {
	url, cleanup := testserver.StartSocketHttpServer(t, requests)

	output := &bytes.Buffer{}
	input := bytes.NewBufferString("input")

	cmd := &Command{
		Config:     &config.Config{GitlabUrl: url},
		Args:       &commandargs.Shell{GitlabKeyId: keyId, SshArgs: []string{"git-receive-pack", "group/repo"}},
		ReadWriter: &readwriter.ReadWriter{ErrOut: output, Out: output, In: input},
	}

	return cmd, output, cleanup
}
