package sshd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"testing"
	"time"

	"github.com/pires/go-proxyproto"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/testhelper"
)

const (
	serverUrl = "127.0.0.1:50000"
	user      = "git"
)

var (
	correlationId = ""
	xForwardedFor = ""
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

func TestListenAndServe_proxyProtocolEnabled(t *testing.T) {
	target, err := net.ResolveTCPAddr("tcp", serverUrl)
	require.NoError(t, err)

	header := &proxyproto.Header{
		Version:           2,
		Command:           proxyproto.PROXY,
		TransportProtocol: proxyproto.TCPv4,
		SourceAddr: &net.TCPAddr{
			IP:   net.ParseIP("10.1.1.1"),
			Port: 1000,
		},
		DestinationAddr: target,
	}
	xForwardedFor = "127.0.0.1"
	defer func() {
		xForwardedFor = "" // Cleanup for other test cases
	}()

	testCases := []struct {
		desc         string
		proxyPolicy  string
		proxyAllowed []string
		header       *proxyproto.Header
		isRejected   bool
	}{
		{
			desc:        "USE (default) without a header",
			proxyPolicy: "",
			header:      nil,
			isRejected:  false,
		},
		{
			desc:        "USE (default) with a header",
			proxyPolicy: "",
			header:      header,
			isRejected:  false,
		},
		{
			desc:        "REQUIRE without a header",
			proxyPolicy: "require",
			header:      nil,
			isRejected:  true,
		},
		{
			desc:        "REQUIRE with a header",
			proxyPolicy: "require",
			header:      header,
			isRejected:  false,
		},
		{
			desc:        "REJECT without a header",
			proxyPolicy: "reject",
			header:      nil,
			isRejected:  false,
		},
		{
			desc:        "REJECT with a header",
			proxyPolicy: "reject",
			header:      header,
			isRejected:  true,
		},
		{
			desc:        "IGNORE without a header",
			proxyPolicy: "ignore",
			header:      nil,
			isRejected:  false,
		},
		{
			desc:        "IGNORE with a header",
			proxyPolicy: "ignore",
			header:      header,
			isRejected:  false,
		},
		{
			desc:         "Allow-listed IP with a header",
			proxyAllowed: []string{"127.0.0.1"},
			header:       header,
			isRejected:   false,
		},
		{
			desc:         "Allow-listed IP without a header",
			proxyAllowed: []string{"127.0.0.1"},
			header:       nil,
			isRejected:   false,
		},
		{
			desc:         "Allow-listed range with a header",
			proxyAllowed: []string{"127.0.0.0/24"},
			header:       header,
			isRejected:   false,
		},
		{
			desc:         "Allow-listed range without a header",
			proxyAllowed: []string{"127.0.0.0/24"},
			header:       nil,
			isRejected:   false,
		},
		{
			desc:         "Not allow-listed IP with a header",
			proxyAllowed: []string{"192.168.1.1"},
			header:       header,
			isRejected:   true,
		},
		{
			desc:         "Not allow-listed IP without a header",
			proxyAllowed: []string{"192.168.1.1"},
			header:       nil,
			isRejected:   false,
		},
		{
			desc:         "Not allow-listed range with a header",
			proxyAllowed: []string{"192.168.1.0/24"},
			header:       header,
			isRejected:   true,
		},
		{
			desc:         "Not allow-listed range without a header",
			proxyAllowed: []string{"192.168.1.0/24"},
			header:       nil,
			isRejected:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			setupServerWithConfig(t, &config.Config{
				Server: config.ServerConfig{
					ProxyProtocol: true,
					ProxyPolicy:   tc.proxyPolicy,
					ProxyAllowed:  tc.proxyAllowed,
				},
			})

			conn, err := net.DialTCP("tcp", nil, target)
			require.NoError(t, err)

			if tc.header != nil {
				_, err := header.WriteTo(conn)
				require.NoError(t, err)
			}

			sshConn, sshChans, sshRequs, err := ssh.NewClientConn(conn, serverUrl, clientConfig(t))
			if sshConn != nil {
				defer sshConn.Close()
			}

			if tc.isRejected {
				require.Error(t, err, "Expected plain SSH request to be failed")
				require.Regexp(t, "ssh: handshake failed", err.Error())
			} else {
				require.NoError(t, err)
				client := ssh.NewClient(sshConn, sshChans, sshRequs)
				defer client.Close()

				holdSession(t, client)
			}
		})
	}
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

func TestInvalidClientConfig(t *testing.T) {
	setupServer(t)

	cfg := clientConfig(t)
	cfg.User = "unknown"
	_, err := ssh.Dial("tcp", serverUrl, cfg)
	require.Error(t, err)
}

func TestInvalidServerConfig(t *testing.T) {
	s := &Server{Config: &config.Config{Server: config.ServerConfig{Listen: "invalid"}}}
	err := s.ListenAndServe(context.Background())

	require.Error(t, err)
	require.Equal(t, "failed to listen for connection: listen tcp: address invalid: missing port in address", err.Error())
	require.Nil(t, s.Shutdown())
}

func TestClosingHangedConnections(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := setupServerWithContext(t, nil, ctx)

	unauthenticatedRequestStatus := make(chan string)
	completed := make(chan bool)

	clientCfg := clientConfig(t)
	clientCfg.HostKeyCallback = func(_ string, _ net.Addr, _ ssh.PublicKey) error {
		unauthenticatedRequestStatus <- "authentication-started"
		<-completed // Wait infinitely

		return nil
	}

	go func() {
		// Start an SSH connection that never ends
		ssh.Dial("tcp", serverUrl, clientCfg)
	}()

	require.Equal(t, "authentication-started", <-unauthenticatedRequestStatus)

	require.NoError(t, s.Shutdown())
	cancel()
	verifyStatus(t, s, StatusClosed)
}

func TestLoginGraceTime(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			LoginGraceTime: config.YamlDuration(50 * time.Millisecond),
		},
	}
	s := setupServerWithConfig(t, cfg)

	unauthenticatedRequestStatus := make(chan string)
	completed := make(chan bool)

	clientCfg := clientConfig(t)
	clientCfg.HostKeyCallback = func(_ string, _ net.Addr, _ ssh.PublicKey) error {
		unauthenticatedRequestStatus <- "authentication-started"
		<-completed // Wait infinitely

		return nil
	}

	go func() {
		// Start an SSH connection that never ends
		ssh.Dial("tcp", serverUrl, clientCfg)
	}()

	require.Equal(t, "authentication-started", <-unauthenticatedRequestStatus)

	// Shutdown the server and verify that it's closed
	// If LoginGraceTime doesn't work, then the connection that runs infinitely, will stop it from closing.
	// The close won't happen until the context is canceled like in TestClosingHangedConnections
	require.NoError(t, s.Shutdown())
	verifyStatus(t, s, StatusClosed)
}

func setupServer(t *testing.T) *Server {
	t.Helper()

	return setupServerWithConfig(t, nil)
}

func setupServerWithConfig(t *testing.T, cfg *config.Config) *Server {
	t.Helper()

	return setupServerWithContext(t, cfg, context.Background())
}

func setupServerWithContext(t *testing.T, cfg *config.Config, ctx context.Context) *Server {
	t.Helper()

	requests := []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/authorized_keys",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				correlationId = r.Header.Get("X-Request-Id")

				require.NotEmpty(t, correlationId)
				require.Equal(t, xForwardedFor, r.Header.Get("X-Forwarded-For"))

				fmt.Fprint(w, `{"id": 1000, "key": "key"}`)
			},
		}, {
			Path: "/api/v4/internal/discover",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, correlationId, r.Header.Get("X-Request-Id"))
				require.Equal(t, xForwardedFor, r.Header.Get("X-Forwarded-For"))

				fmt.Fprint(w, `{"id": 1000, "name": "Test User", "username": "test-user"}`)
			},
		},
	}

	testhelper.PrepareTestRootDir(t)

	url := testserver.StartSocketHttpServer(t, requests)

	if cfg == nil {
		cfg = &config.Config{}
	}

	// All things that don't need to be configurable in tests yet
	cfg.GitlabUrl = url
	cfg.RootDir = "/tmp"
	cfg.User = user
	cfg.Server.Listen = serverUrl
	cfg.Server.ConcurrentSessionsLimit = 1
	cfg.Server.HostKeyFiles = []string{path.Join(testhelper.TestRoot, "certs/valid/server.key")}

	s, err := NewServer(cfg)
	require.NoError(t, err)

	go func() { require.NoError(t, s.ListenAndServe(ctx)) }()
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
	require.Eventually(t, func() bool { return s.getStatus() == st }, 2*time.Second, time.Millisecond)
}
