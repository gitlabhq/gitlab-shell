package uploadarchive

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/labkit/correlation"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/testhelper/gitalytest"
)

func TestUploadArchive(t *testing.T) {
	gitalytest.RunAllNetworks(t, func(t *testing.T, servers gitalytest.Servers) {
		correlationID := correlation.SafeRandomID()
		setup := gitalytest.NewCommandSetup(gitalytest.CommandOptions{
			GitlabURL:     servers.GitlabURL,
			CommandType:   commandargs.UploadArchive,
			Repository:    "group/repo",
			RemoteAddr:    "127.0.0.1",
			GitlabKeyID:   "1",
			CorrelationID: correlationID,
			ClientName:    "gitlab-shell-tests",
		})
		cmd := &Command{Config: setup.Config, Args: setup.Args, ReadWriter: setup.ReadWriter}
		_, err := cmd.Execute(setup.Context)
		require.NoError(t, err)
		require.Equal(t, "UploadArchive: group/repo", setup.Output.String())
		gitalytest.RequireMetadata(t, servers.Gitaly, gitalytest.MetadataExpectations{
			ClientNameSuffix: "git-upload-archive",
			CorrelationID:    correlationID,
			RemoteAddr:       "127.0.0.1",
		})
	})
}
