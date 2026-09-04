// Package gitalytest provides shared setup and assertions for Gitaly integration tests.
package gitalytest

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/labkit/correlation"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/sshenv"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/testhelper/requesthandlers"
)

var networks = []string{"unix", "tcp", "dns"}

// Servers contains the test services started for a network subtest.
type Servers struct {
	GitlabURL string
	Gitaly    *testserver.TestGitalyServer
}

// RunAllNetworks runs a test callback against Unix, TCP, and DNS Gitaly addresses.
func RunAllNetworks(t *testing.T, run func(t *testing.T, servers Servers)) {
	t.Helper()

	for _, network := range networks {
		t.Run(fmt.Sprintf("via %s network", network), func(t *testing.T) {
			gitalyAddress, gitalyServer := testserver.StartGitalyServer(t, network)
			t.Logf("Server address: %s", gitalyAddress)

			requests := requesthandlers.BuildAllowedWithGitalyHandlers(t, gitalyAddress)
			gitlabURL := testserver.StartHTTPServer(t, requests)

			run(t, Servers{
				GitlabURL: gitlabURL,
				Gitaly:    gitalyServer,
			})
		})
	}
}

// CommandOptions defines the command test setup inputs.
type CommandOptions struct {
	GitlabURL      string
	CommandType    commandargs.CommandType
	Repository     string
	RemoteAddr     string
	GitlabUsername string
	GitlabKeyID    string
	CorrelationID  string
	ClientName     string
}

// CommandSetup contains the initialized dependencies for a command test.
type CommandSetup struct {
	Context    context.Context
	Config     *config.Config
	Args       *commandargs.Shell
	ReadWriter *readwriter.ReadWriter
	Output     *bytes.Buffer
}

// NewCommandSetup initializes command arguments, context, configuration, and I/O.
func NewCommandSetup(options CommandOptions) CommandSetup {
	input := &bytes.Buffer{}
	output := &bytes.Buffer{}
	executable := string(options.CommandType)
	env := sshenv.Env{
		IsSSHConnection: true,
		OriginalCommand: executable + " " + options.Repository,
		RemoteAddr:      options.RemoteAddr,
	}
	args := &commandargs.Shell{
		GitlabUsername: options.GitlabUsername,
		GitlabKeyID:    options.GitlabKeyID,
		CommandType:    options.CommandType,
		SSHArgs:        []string{executable, options.Repository},
		Env:            env,
	}
	ctx := correlation.ContextWithCorrelation(context.Background(), options.CorrelationID)
	ctx = correlation.ContextWithClientName(ctx, options.ClientName)
	cfg := &config.Config{GitlabURL: options.GitlabURL}
	cfg.GitalyClient.InitSidechannelRegistry(ctx)
	readWriter := &readwriter.ReadWriter{ErrOut: output, Out: output, In: input}

	return CommandSetup{
		Context:    ctx,
		Config:     cfg,
		Args:       args,
		ReadWriter: readWriter,
		Output:     output,
	}
}

// MetadataExpectations defines expected Gitaly request metadata.
type MetadataExpectations struct {
	ClientNameSuffix string
	CorrelationID    string
	RemoteAddr       string
}

// RequireMetadata asserts the common and command-specific Gitaly request metadata.
func RequireMetadata(t *testing.T, server *testserver.TestGitalyServer, expected MetadataExpectations) {
	t.Helper()

	expectedMetadata := map[string]string{
		"gitaly-feature-cache_invalidator":        "true",
		"gitaly-feature-inforef_uploadpack_cache": "false",
		"x-gitlab-client-name":                    "gitlab-shell-tests-" + expected.ClientNameSuffix,
		"key_id":                                  "123",
		"user_id":                                 "user-1",
		"remote_ip":                               expected.RemoteAddr,
		"key_type":                                "key",
	}
	for key, value := range expectedMetadata {
		actual := server.ReceivedMD[key]
		require.Len(t, actual, 1)
		require.Equal(t, value, actual[0])
	}
	require.Empty(t, server.ReceivedMD["some-other-feature-flag"])

	actualCorrelationID := server.ReceivedMD["x-gitlab-correlation-id"]
	require.Len(t, actualCorrelationID, 1)
	require.Equal(t, expected.CorrelationID, actualCorrelationID[0])
}
