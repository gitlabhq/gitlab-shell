// Package sshd provides functionality for handling SSH connections
package sshd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	proxyproto "github.com/pires/go-proxyproto"
	"golang.org/x/crypto/ssh"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/metrics"

	"gitlab.com/gitlab-org/labkit/correlation"
	"gitlab.com/gitlab-org/labkit/log"
)

type status int

const (
	// StatusStarting represents the starting status of the SSH server
	StatusStarting status = iota

	// StatusReady represents the ready status of the SSH server
	StatusReady

	// StatusOnShutdown represents the status when the SSH server is shutting down
	StatusOnShutdown

	// StatusClosed represents the closed status of the SSH server
	StatusClosed
)

// Server represents an SSH server instance
type Server struct {
	Config *config.Config

	status       status
	statusMu     sync.RWMutex
	wg           sync.WaitGroup
	listener     net.Listener
	serverConfig *serverConfig
}

type logInfo struct{}

// NewServer creates a new instance of Server
func NewServer(cfg *config.Config) (*Server, error) {
	serverConfig, err := newServerConfig(cfg)
	if err != nil {
		return nil, err
	}

	return &Server{Config: cfg, serverConfig: serverConfig}, nil
}

// ListenAndServe starts listening for SSH connections and serves them
func (s *Server) ListenAndServe(ctx context.Context) error {
	if err := s.listen(ctx); err != nil {
		return err
	}
	defer func() { _ = s.listener.Close() }()

	s.serve(ctx)

	return nil
}

// Shutdown gracefully shuts down the SSH server
func (s *Server) Shutdown() error {
	if s.listener == nil {
		return nil
	}

	s.changeStatus(StatusOnShutdown)

	return s.listener.Close()
}

// MonitoringServeMux returns the ServeMux for monitoring endpoints
func (s *Server) MonitoringServeMux() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc(s.Config.Server.ReadinessProbe, func(w http.ResponseWriter, _ *http.Request) {
		if s.getStatus() == StatusReady {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	})

	mux.HandleFunc(s.Config.Server.LivenessProbe, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	return mux
}

func (s *Server) listen(ctx context.Context) error {
	sshListener, err := net.Listen("tcp", s.Config.Server.Listen)
	if err != nil {
		return fmt.Errorf("failed to listen for connection: %w", err)
	}

	if s.Config.Server.ProxyProtocol {
		policy, err := s.proxyPolicy()
		if err != nil {
			return fmt.Errorf("invalid policy configuration: %w", err)
		}

		sshListener = &proxyproto.Listener{
			Listener:          sshListener,
			Policy:            policy,
			ReadHeaderTimeout: time.Duration(s.Config.Server.ProxyHeaderTimeout),
		}

		log.ContextLogger(ctx).Info("Proxy protocol is enabled")
	}

	fields := log.Fields{
		"tcp_address": sshListener.Addr().String(),
	}

	if len(s.serverConfig.cfg.Server.PublicKeyAlgorithms) > 0 {
		fields["supported_public_key_algorithms"] = s.serverConfig.cfg.Server.PublicKeyAlgorithms
	}

	log.WithContextFields(ctx, fields).Info("Listening for SSH connections")

	s.listener = sshListener

	return nil
}

func (s *Server) serve(ctx context.Context) {
	s.changeStatus(StatusReady)

	for {
		nconn, err := s.listener.Accept()
		if err != nil {
			if s.getStatus() == StatusOnShutdown {
				break
			}

			log.ContextLogger(ctx).WithError(err).Warn("Failed to accept connection")
			continue
		}

		s.wg.Add(1)
		go s.handleConn(ctx, nconn)
	}

	s.wg.Wait()

	s.changeStatus(StatusClosed)
}

func (s *Server) changeStatus(st status) {
	s.statusMu.Lock()
	s.status = st
	s.statusMu.Unlock()
}

func (s *Server) getStatus() status {
	s.statusMu.RLock()
	defer s.statusMu.RUnlock()

	return s.status
}

func contextWithValues(parent context.Context, nconn net.Conn) context.Context {
	ctx := correlation.ContextWithCorrelation(parent, correlation.SafeRandomID())

	// If we're dealing with a PROXY connection, register the original requester's IP
	mconn, ok := nconn.(*proxyproto.Conn)
	if ok {
		ip := gitlabnet.ParseIP(mconn.Raw().RemoteAddr().String())
		ctx = context.WithValue(ctx, client.OriginalRemoteIPContextKey{}, ip)
	}

	return ctx
}

func (s *Server) handleConn(ctx context.Context, nconn net.Conn) {
	defer s.wg.Done()

	metrics.SshdConnectionsInFlight.Inc()
	defer metrics.SshdConnectionsInFlight.Dec()

	ctx, cancel := context.WithCancel(contextWithValues(ctx, nconn))
	defer cancel()
	go func() {
		<-ctx.Done()
		_ = nconn.Close() // Close the connection when context is canceled
	}()

	remoteAddr := nconn.RemoteAddr().String()
	ctxlog := log.WithContextFields(ctx, log.Fields{"remote_addr": remoteAddr})

	// Prevent a panic in a single connection from taking out the whole server
	defer func() {
		if err := recover(); err != nil {
			ctxlog.WithField("recovered_error", err).Error("panic handling session")

			metrics.SliSshdSessionsErrorsTotal.Inc()
		}
	}()

	started := time.Now()
	conn := newConnection(s.Config, nconn)

	var ctxWithLogData context.Context

	conn.handle(ctx, s.serverConfig.get(ctx), func(ctx context.Context, sconn *ssh.ServerConn, channel ssh.Channel, requests <-chan *ssh.Request) error {
		session := &session{
			cfg:                 s.Config,
			channel:             channel,
			gitlabKeyID:         sconn.Permissions.Extensions["key-id"],
			gitlabKrb5Principal: sconn.Permissions.Extensions["krb5principal"],
			gitlabUsername:      sconn.Permissions.Extensions["username"],
			namespace:           sconn.Permissions.Extensions["namespace"],
			remoteAddr:          remoteAddr,
			started:             time.Now(),
		}

		var err error
		ctxWithLogData, err = session.handle(ctx, requests)

		return err
	})

	logData := extractLogDataFromContext(ctxWithLogData)

	ctxlog.WithFields(log.Fields{
		"duration_s":    time.Since(started).Seconds(),
		"written_bytes": logData.WrittenBytes,
		"meta":          logData.Meta,
	}).Info("access: finish")
}

func (s *Server) proxyPolicy() (proxyproto.PolicyFunc, error) {
	if len(s.Config.Server.ProxyAllowed) > 0 {
		return proxyproto.StrictWhiteListPolicy(s.Config.Server.ProxyAllowed)
	}

	// Set the Policy value based on config
	// Values are taken from https://github.com/pires/go-proxyproto/blob/195fedcfbfc1be163f3a0d507fac1709e9d81fed/policy.go#L20
	switch strings.ToLower(s.Config.Server.ProxyPolicy) {
	case "require":
		return staticProxyPolicy(proxyproto.REQUIRE), nil
	case "ignore":
		return staticProxyPolicy(proxyproto.IGNORE), nil
	case "reject":
		return staticProxyPolicy(proxyproto.REJECT), nil
	default:
		return staticProxyPolicy(proxyproto.USE), nil
	}
}

func extractDataFromContext(ctx context.Context) command.LogData {
	logData := command.LogData{}

	if ctx == nil {
		return logData
	}

	if ctx.Value("logData") != nil {
		logData = ctx.Value("logData").(command.LogData)
	}

	return logData
}

func extractLogDataFromContext(ctx context.Context) command.LogData {
	logData := command.LogData{}

	if ctx == nil {
		return logData
	}

	if ctx.Value(logInfo{}) != nil {
		logData = ctx.Value(logInfo{}).(command.LogData)
	}

	return logData
}

func staticProxyPolicy(policy proxyproto.Policy) proxyproto.PolicyFunc {
	return func(_ net.Addr) (proxyproto.Policy, error) {
		return policy, nil
	}
}
