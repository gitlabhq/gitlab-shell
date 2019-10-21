package uploadarchive

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
	url, cleanup := testserver.StartHttpServer(t, requests)
	defer cleanup()

	output := &bytes.Buffer{}

	cmd := &Command{
		Config:     &config.Config{GitlabUrl: url},
		Args:       &commandargs.Shell{GitlabKeyId: "disallowed", SshArgs: []string{"git-upload-archive", "group/repo"}},
		ReadWriter: &readwriter.ReadWriter{ErrOut: output, Out: output},
	}

	err := cmd.Execute()
	require.Equal(t, "Disallowed by API call", err.Error())
}
