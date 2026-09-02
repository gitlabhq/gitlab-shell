package uploadpack

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/testhelper/gitalytest"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/testhelper/requesthandlers"
)

const (
	testRepo          = "group/repo"
	testRemoteAddr    = "127.0.0.1"
	testGitUploadPack = "git-upload-pack"
)

func TestUploadPack(t *testing.T) {
	gitalytest.RunAllNetworks(t, func(t *testing.T, servers gitalytest.Servers) {
		setup := gitalytest.NewCommandSetup(gitalytest.CommandOptions{
			GitlabURL:     servers.GitlabURL,
			CommandType:   commandargs.UploadPack,
			Repository:    testRepo,
			RemoteAddr:    testRemoteAddr,
			GitlabKeyID:   "1",
			CorrelationID: "a-correlation-id",
			ClientName:    "gitlab-shell-tests",
		})
		cmd := &Command{Config: setup.Config, Args: setup.Args, ReadWriter: setup.ReadWriter}
		_, err := cmd.Execute(setup.Context)
		require.NoError(t, err)
		require.Equal(t, "SSHUploadPackWithSidechannel: "+testRepo, setup.Output.String())
		gitalytest.RequireMetadata(t, servers.Gitaly, gitalytest.MetadataExpectations{
			ClientNameSuffix: "git-upload-pack",
			CorrelationID:    "a-correlation-id",
			RemoteAddr:       testRemoteAddr,
		})
	})
}

func TestUploadPackWithRetryConfig(t *testing.T) {
	gitalyAddress, testServer := testserver.StartGitalyServer(t, "tcp")

	retryConfig := map[string]interface{}{
		"maxAttempts":          4,
		"initialBackoff":       "0.1s",
		"maxBackoff":           "1s",
		"backoffMultiplier":    2,
		"retryableStatusCodes": []string{"UNAVAILABLE"},
	}
	requests := requesthandlers.BuildAllowedWithGitalyHandlersAndRetryConfig(t, gitalyAddress, retryConfig)
	url := testserver.StartHTTPServer(t, requests)

	setup := gitalytest.NewCommandSetup(gitalytest.CommandOptions{
		GitlabURL:     url,
		CommandType:   commandargs.UploadPack,
		Repository:    testRepo,
		RemoteAddr:    testRemoteAddr,
		GitlabKeyID:   "1",
		CorrelationID: "retry-test",
		ClientName:    "gitlab-shell-tests",
	})
	cmd := &Command{Config: setup.Config, Args: setup.Args, ReadWriter: setup.ReadWriter}
	_, err := cmd.Execute(setup.Context)
	require.NoError(t, err)

	require.Equal(t, "SSHUploadPackWithSidechannel: "+testRepo, setup.Output.String())
	correlationIDs := testServer.ReceivedMD["x-gitlab-correlation-id"]
	require.Len(t, correlationIDs, 1)
	require.Equal(t, "retry-test", correlationIDs[0])
}
