package sshd

import (
	"testing"
	"context"
	"path"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/testhelper"
)

const serverUrl = "127.0.0.1:50000"

func TestShutdown(t *testing.T) {
	s := setupServer(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		require.NoError(t, s.serve(ctx))
		done <- true
	}()

	require.NoError(t, s.Shutdown())

	require.True(t, <-done, "the accepting loop must be interrupted")
}

func setupServer(t *testing.T) *Server {
	testhelper.PrepareTestRootDir(t)

	url := testserver.StartSocketHttpServer(t, []testserver.TestRequestHandler{})
	srvCfg := config.ServerConfig{
		Listen: serverUrl,
		HostKeyFiles: []string{path.Join(testhelper.TestRoot, "certs/valid/server.key")},
	}

	cfg := &config.Config{RootDir: "/tmp", GitlabUrl: url, Server: srvCfg}

	s := &Server{Config: cfg}
	require.NoError(t, s.listen())

	return s
}
