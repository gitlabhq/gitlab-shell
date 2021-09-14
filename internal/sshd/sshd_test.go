package sshd

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"gitlab.com/gitlab-org/gitlab-shell/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/testhelper"
)

const (
	serverUrl = "127.0.0.1:50000"
	user      = "git"
)

var (
	correlationId = ""
)

func TestListenAndServe(t *testing.T) {
	s := setupServer(t)

	client, err := ssh.Dial("tcp", serverUrl, clientConfig(t))
	require.NoError(t, err)
	defer client.Close()

	require.NoError(t, s.Shutdown())
	verifyStatus(t, s, StatusOnShutdown)

	holdSession(t, client)

	_, err = ssh.Dial("tcp", serverUrl, clientConfig(t))
	require.Equal(t, err.Error(), "dial tcp 127.0.0.1:50000: connect: connection refused")

	client.Close()

	verifyStatus(t, s, StatusClosed)
}

func TestCorrelationId(t *testing.T) {
	setupServer(t)

	client, err := ssh.Dial("tcp", serverUrl, clientConfig(t))
	require.NoError(t, err)
	defer client.Close()

	holdSession(t, client)

	previousCorrelationId := correlationId

	client, err = ssh.Dial("tcp", serverUrl, clientConfig(t))
	require.NoError(t, err)
	defer client.Close()

	holdSession(t, client)

	require.NotEqual(t, previousCorrelationId, correlationId)
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

func TestNewServerWithoutHosts(t *testing.T) {
	_, err := NewServer(&config.Config{GitlabUrl: "http://localhost"})

	require.Error(t, err)
	require.Equal(t, "No host keys could be loaded, aborting", err.Error())
}

func TestInvalidClientConfig(t *testing.T) {
	setupServer(t)

	cfg := clientConfig(t)
	cfg.User = "unknown"
	_, err := ssh.Dial("tcp", serverUrl, cfg)
	require.Error(t, err)
}

func setupServer(t *testing.T) *Server {
	t.Helper()

	requests := []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/authorized_keys",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				correlationId = r.Header.Get("X-Request-Id")

				require.NotEmpty(t, correlationId)

				fmt.Fprint(w, `{"id": 1000, "key": "key"}`)
			},
		}, {
			Path: "/api/v4/internal/discover",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, correlationId, r.Header.Get("X-Request-Id"))

				fmt.Fprint(w, `{"id": 1000, "name": "Test User", "username": "test-user"}`)
			},
		},
	}

	testhelper.PrepareTestRootDir(t)

	url := testserver.StartSocketHttpServer(t, requests)
	srvCfg := config.ServerConfig{
		Listen:                  serverUrl,
		ConcurrentSessionsLimit: 1,
		HostKeyFiles:            []string{path.Join(testhelper.TestRoot, "certs/valid/server.key")},
	}

	s, err := NewServer(&config.Config{User: user, RootDir: "/tmp", GitlabUrl: url, Server: srvCfg})
	require.NoError(t, err)

	go func() { require.NoError(t, s.ListenAndServe(context.Background())) }()
	t.Cleanup(func() { s.Shutdown() })

	verifyStatus(t, s, StatusReady)

	return s
}

func clientConfig(t *testing.T) *ssh.ClientConfig {
	keyRaw, err := os.ReadFile(path.Join(testhelper.TestRoot, "certs/valid/server_authorized_key"))
	pKey, _, _, _, err := ssh.ParseAuthorizedKey(keyRaw)
	require.NoError(t, err)

	key, err := os.ReadFile(path.Join(testhelper.TestRoot, "certs/client/key.pem"))
	require.NoError(t, err)
	signer, err := ssh.ParsePrivateKey(key)
	require.NoError(t, err)

	return &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.FixedHostKey(pKey),
	}
}

func holdSession(t *testing.T, c *ssh.Client) {
	session, err := c.NewSession()
	require.NoError(t, err)
	defer session.Close()

	output, err := session.Output("discover")
	require.NoError(t, err)
	require.Equal(t, "Welcome to GitLab, @test-user!\n", string(output))
}

func verifyStatus(t *testing.T, s *Server, st status) {
	for i := 5; i < 500; i += 50 {
		if s.getStatus() == st {
			break
		}

		// Sleep incrementally ~2s in total
		time.Sleep(time.Duration(i) * time.Millisecond)
	}

	require.Equal(t, st, s.getStatus())
}
