package uploadpack

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/testhelper/requesthandlers"
)

func TestForbiddenAccess(t *testing.T) {
	requests := requesthandlers.BuildDisallowedByApiHandlers(t)
	url := testserver.StartHttpServer(t, requests)

	output := &bytes.Buffer{}

	cmd := &Command{
		Config:     &config.Config{GitlabUrl: url},
		Args:       &commandargs.Shell{GitlabKeyId: "disallowed", SshArgs: []string{"git-upload-pack", "group/repo"}},
		ReadWriter: &readwriter.ReadWriter{ErrOut: output, Out: output},
	}

	err := cmd.Execute(context.Background())
	require.Equal(t, "Disallowed by API call", err.Error())
}
