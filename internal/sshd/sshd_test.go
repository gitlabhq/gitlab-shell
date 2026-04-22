package sshd

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"testing"
	"time"

	"github.com/pires/go-proxyproto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/testhelper"
	"gitlab.com/gitlab-org/labkit/v2/log"
)

const (
	user = "git"
)

var (
	correlationID = ""
	xForwardedFor = ""
)

func TestListenAndServe(t *testing.T) {
	s, testRoot := setupServer(t)

	client, err := ssh.Dial("tcp", s.Addr(), clientConfig(t, testRoot))
	require.NoError(t, err)
	defer client.Close()

	require.NoError(t, s.Shutdown())
	verifyStatus(t, s, StatusOnShutdown)

	holdSession(t, client)

	_, err = ssh.Dial("tcp", s.Addr(), clientConfig(t, testRoot))
	require.Error(t, err)
	require.Contains(t, err.Error(), "connection refused")

	client.Close()

	verifyStatus(t, s, StatusClosed)
}

func TestListenAndServe_proxyProtocolEnabled(t *testing.T) {
	xForwardedFor = "127.0.0.1"
	defer func() {
		xForwardedFor = "" // Cleanup for other test cases
	}()

	testCases := []struct {
		desc         string
		proxyPolicy  string
		proxyAllowed []string
		sendHeader   bool
		isRejected   bool
	}{
		{
			desc:        "USE (default) without a header",
			proxyPolicy: "",
			sendHeader:  false,
			isRejected:  false,
		},
		{
			desc:        "USE (default) with a header",
			proxyPolicy: "",
			sendHeader:  true,
			isRejected:  false,
		},
		{
			desc:        "REQUIRE without a header",
			proxyPolicy: "require",
			sendHeader:  false,
			isRejected:  true,
		},
		{
			desc:        "REQUIRE with a header",
			proxyPolicy: "require",
			sendHeader:  true,
			isRejected:  false,
		},
		{
			desc:        "REJECT without a header",
			proxyPolicy: "reject",
			sendHeader:  false,
			isRejected:  false,
		},
		{
			desc:        "REJECT with a header",
			proxyPolicy: "reject",
			sendHeader:  true,
			isRejected:  true,
		},
		{
			desc:        "IGNORE without a header",
			proxyPolicy: "ignore",
			sendHeader:  false,
			isRejected:  false,
		},
		{
			desc:        "IGNORE with a header",
			proxyPolicy: "ignore",
			sendHeader:  true,
			isRejected:  false,
		},
		{
			desc:         "Allow-listed IP with a header",
			proxyAllowed: []string{"127.0.0.1"},
			sendHeader:   true,
			isRejected:   false,
		},
		{
			desc:         "Allow-listed IP without a header",
			proxyAllowed: []string{"127.0.0.1"},
			sendHeader:   false,
			isRejected:   false,
		},
		{
			desc:         "Allow-listed range with a header",
			proxyAllowed: []string{"127.0.0.0/24"},
			sendHeader:   true,
			isRejected:   false,
		},
		{
			desc:         "Allow-listed range without a header",
			proxyAllowed: []string{"127.0.0.0/24"},
			sendHeader:   false,
			isRejected:   false,
		},
		{
			desc:         "Not allow-listed IP with a header",
			proxyAllowed: []string{"192.168.1.1"},
			sendHeader:   true,
			isRejected:   true,
		},
		{
			desc:         "Not allow-listed IP without a header",
			proxyAllowed: []string{"192.168.1.1"},
			sendHeader:   false,
			isRejected:   false,
		},
		{
			desc:         "Not allow-listed range with a header",
			proxyAllowed: []string{"192.168.1.0/24"},
			sendHeader:   true,
			isRejected:   true,
		},
		{
			desc:         "Not allow-listed range without a header",
			proxyAllowed: []string{"192.168.1.0/24"},
			sendHeader:   false,
			isRejected:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			s, testRoot := setupServerWithConfig(t, &config.Config{
				Server: config.ServerConfig{
					ProxyProtocol: true,
					ProxyPolicy:   tc.proxyPolicy,
					ProxyAllowed:  tc.proxyAllowed,
				},
			})

			serverAddr := s.Addr()
			target, err := net.ResolveTCPAddr("tcp", serverAddr)
			require.NoError(t, err)

			conn, err := net.DialTCP("tcp", nil, target)
			require.NoError(t, err)
			defer conn.Close()

			if tc.sendHeader {
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
				_, writeToErr := header.WriteTo(conn)
				require.NoError(t, writeToErr)
			}

			sshConn, sshChans, sshRequs, err := ssh.NewClientConn(conn, serverAddr, clientConfig(t, testRoot))
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
	s, testRoot := setupServer(t)

	client, err := ssh.Dial("tcp", s.Addr(), clientConfig(t, testRoot))
	require.NoError(t, err)
	defer client.Close()

	holdSession(t, client)

	previousCorrelationID := correlationID

	client, err = ssh.Dial("tcp", s.Addr(), clientConfig(t, testRoot))
	require.NoError(t, err)
	defer client.Close()

	holdSession(t, client)

	require.NotEqual(t, previousCorrelationID, correlationID)
}

func TestReadinessProbe(t *testing.T) {
	s := &Server{Config: &config.Config{Server: config.DefaultServerConfig}}

	require.Equal(t, StatusStarting, s.getStatus())

	mux := s.MonitoringServeMux()

	req := httptest.NewRequest("GET", "/start", nil)

	r := httptest.NewRecorder()
	mux.ServeHTTP(r, req)
	res := r.Result()
	require.Equal(t, 503, res.StatusCode)
	res.Body.Close()

	s.changeStatus(StatusReady)

	r = httptest.NewRecorder()
	mux.ServeHTTP(r, req)
	res = r.Result()
	require.Equal(t, 200, res.StatusCode)
	res.Body.Close()

	s.changeStatus(StatusOnShutdown)

	r = httptest.NewRecorder()
	mux.ServeHTTP(r, req)
	res = r.Result()
	require.Equal(t, 503, res.StatusCode)
	res.Body.Close()
}

func TestLivenessProbe(t *testing.T) {
	s := &Server{Config: &config.Config{Server: config.DefaultServerConfig}}
	mux := s.MonitoringServeMux()

	req := httptest.NewRequest("GET", "/health", nil)

	r := httptest.NewRecorder()
	mux.ServeHTTP(r, req)
	res := r.Result()
	require.Equal(t, 200, res.StatusCode)
	res.Body.Close()
}

func TestInvalidClientConfig(t *testing.T) {
	s, testRoot := setupServer(t)

	cfg := clientConfig(t, testRoot)
	cfg.User = "unknown"
	_, err := ssh.Dial("tcp", s.Addr(), cfg)
	require.Error(t, err)
}

func TestInvalidServerConfig(t *testing.T) {
	s := &Server{Config: &config.Config{Server: config.ServerConfig{Listen: "invalid"}}}
	err := s.ListenAndServe(context.Background())

	require.Error(t, err)
	require.Equal(t, "failed to listen for connection: listen tcp: address invalid: missing port in address", err.Error())
	require.NoError(t, s.Shutdown())
}

func TestClosingHangedConnections(t *testing.T) {
	ctx, cancel := context.WithCancel(log.WithLogger(context.Background(), slog.Default()))
	defer cancel()

	s, testRoot := setupServerWithContext(ctx, t, nil)

	unauthenticatedRequestStatus := make(chan string)
	completed := make(chan bool)

	clientCfg := clientConfig(t, testRoot)
	clientCfg.HostKeyCallback = func(_ string, _ net.Addr, _ ssh.PublicKey) error {
		unauthenticatedRequestStatus <- "authentication-started"
		<-completed // Wait infinitely

		return nil
	}

	go func() {
		// Start an SSH connection that never ends
		ssh.Dial("tcp", s.Addr(), clientCfg)
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
	s, testRoot := setupServerWithConfig(t, cfg)

	unauthenticatedRequestStatus := make(chan string)
	completed := make(chan bool)

	clientCfg := clientConfig(t, testRoot)
	clientCfg.HostKeyCallback = func(_ string, _ net.Addr, _ ssh.PublicKey) error {
		unauthenticatedRequestStatus <- "authentication-started"
		<-completed // Wait infinitely

		return nil
	}

	go func() {
		// Start an SSH connection that never ends
		ssh.Dial("tcp", s.Addr(), clientCfg)
	}()

	require.Equal(t, "authentication-started", <-unauthenticatedRequestStatus)

	// Shutdown the server and verify that it's closed
	// If LoginGraceTime doesn't work, then the connection that runs infinitely, will stop it from closing.
	// The close won't happen until the context is canceled like in TestClosingHangedConnections
	require.NoError(t, s.Shutdown())
	verifyStatus(t, s, StatusClosed)
}

func TestExtractMetaDataFromContext(t *testing.T) {
	username := "alex-doe"
	rootNameSpace := "flightjs"
	project := fmt.Sprintf("%s/Flight", rootNameSpace)
	projectID := 1
	rootNamespaceID := 2
	ctx := context.WithValue(context.Background(), logInfo{}, command.NewLogData(project, username, projectID, rootNamespaceID))

	data := extractLogDataFromContext(ctx)

	require.Equal(t, command.LogData{Username: username, Meta: command.LogMetadata{Project: project, RootNamespace: rootNameSpace, ProjectID: projectID, RootNamespaceID: rootNamespaceID}}, data)
}

func TestExtractMetaDataFromContextWithoutMetaData(t *testing.T) {
	data := extractLogDataFromContext(context.Background())

	require.Equal(t, command.LogData{}, data)
}

func TestExtractMetaDataFromNilContext(t *testing.T) {
	var ctx context.Context

	data := extractLogDataFromContext(ctx)

	require.Equal(t, command.LogData{}, data)
}

func setupServer(t *testing.T) (*Server, string) {
	t.Helper()

	return setupServerWithConfig(t, nil)
}

func setupServerWithConfig(t *testing.T, cfg *config.Config) (*Server, string) {
	t.Helper()

	return setupServerWithContext(log.WithLogger(context.Background(), slog.Default()), t, cfg)
}

func setupServerWithContext(ctx context.Context, t *testing.T, cfg *config.Config) (*Server, string) {
	t.Helper()

	testRoot := testhelper.PrepareTestRootDir(t)

	requests := []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/authorized_keys",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				correlationID = r.Header.Get("X-Request-Id")

				assert.NotEmpty(t, correlationID)
				assert.Equal(t, xForwardedFor, r.Header.Get("X-Forwarded-For"))

				fmt.Fprint(w, `{"id": 1000, "key": "key"}`)
			},
		}, {
			Path: "/api/v4/internal/discover",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, correlationID, r.Header.Get("X-Request-Id"))
				assert.Equal(t, xForwardedFor, r.Header.Get("X-Forwarded-For"))

				fmt.Fprint(w, `{"id": 1000, "name": "Test User", "username": "test-user"}`)
			},
		},
	}

	url := testserver.StartSocketHTTPServer(t, requests)

	if cfg == nil {
		cfg = &config.Config{}
	}

	// All things that don't need to be configurable in tests yet
	cfg.GitlabURL = url
	cfg.RootDir = "/tmp"
	cfg.User = user
	cfg.Server.Listen = "127.0.0.1:0"
	cfg.Server.ConcurrentSessionsLimit = 1
	cfg.Server.HostKeyFiles = []string{path.Join(testRoot, "certs/valid/server.key")}

	s, err := NewServer(cfg)
	require.NoError(t, err)

	go func() { s.ListenAndServe(ctx) }()
	//nolint:godox // NOTE: Changing the below to { require.NoError(t, s.Shutdown()) } results in failed tests
	t.Cleanup(func() { s.Shutdown() })

	verifyStatus(t, s, StatusReady)

	return s, testRoot
}

func clientConfig(t *testing.T, testRoot string) *ssh.ClientConfig {
	keyRaw, _ := os.ReadFile(path.Join(testRoot, "certs/valid/server_authorized_key"))
	pKey, _, _, _, err := ssh.ParseAuthorizedKey(keyRaw) //nolint:dogsled
	require.NoError(t, err)

	key, err := os.ReadFile(path.Join(testRoot, "certs/client/key.pem"))
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
