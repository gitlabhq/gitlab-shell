package sshd

import (
	"context"
	"net/http/httptest"
	"path"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/testhelper"
)

const serverUrl = "127.0.0.1:50000"

func TestShutdown(t *testing.T) {
	s := setupServer(t)

	go func() { require.NoError(t, s.ListenAndServe(context.Background())) }()

	verifyStatus(t, s, StatusReady)

	s.wg.Add(1)

	require.NoError(t, s.Shutdown())
	verifyStatus(t, s, StatusOnShutdown)

	s.wg.Done()

	verifyStatus(t, s, StatusClosed)
}

func TestReadinessProbe(t *testing.T) {
	s := &Server{Config: &config.Config{Server: config.DefaultServerConfig}}

	require.Equal(t, StatusStarting, s.getStatus())

	mux := s.MonitoringServeMux()

	req := httptest.NewRequest("GET", "/start", nil)

	r := httptest.NewRecorder()
	mux.ServeHTTP(r, req)
	require.Equal(t, 503, r.Result().StatusCode)

	s.changeStatus(StatusReady)

	r = httptest.NewRecorder()
	mux.ServeHTTP(r, req)
	require.Equal(t, 200, r.Result().StatusCode)

	s.changeStatus(StatusOnShutdown)

	r = httptest.NewRecorder()
	mux.ServeHTTP(r, req)
	require.Equal(t, 503, r.Result().StatusCode)
}

func TestLivenessProbe(t *testing.T) {
	s := &Server{Config: &config.Config{Server: config.DefaultServerConfig}}
	mux := s.MonitoringServeMux()

	req := httptest.NewRequest("GET", "/health", nil)

	r := httptest.NewRecorder()
	mux.ServeHTTP(r, req)
	require.Equal(t, 200, r.Result().StatusCode)
}

func setupServer(t *testing.T) *Server {
	testhelper.PrepareTestRootDir(t)

	url := testserver.StartSocketHttpServer(t, []testserver.TestRequestHandler{})
	srvCfg := config.ServerConfig{
		Listen:       serverUrl,
		HostKeyFiles: []string{path.Join(testhelper.TestRoot, "certs/valid/server.key")},
	}

	cfg := &config.Config{RootDir: "/tmp", GitlabUrl: url, Server: srvCfg}

	return &Server{Config: cfg}
}

func verifyStatus(t *testing.T, s *Server, st status) {
	for i := 5; i < 500; i += 50 {
		if s.getStatus() == st {
			break
		}

		// Sleep incrementally ~2s in total
		time.Sleep(time.Duration(i) * time.Millisecond)
	}

	require.Equal(t, s.getStatus(), st)
}
