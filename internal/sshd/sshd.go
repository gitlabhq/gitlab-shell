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
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/metrics"

	"gitlab.com/gitlab-org/labkit/correlation"
	"gitlab.com/gitlab-org/labkit/log"
)

type status int

const (
	StatusStarting status = iota
	StatusReady
	StatusOnShutdown
	StatusClosed
)

type Server struct {
	Config *config.Config

	status       status
	statusMu     sync.RWMutex
	wg           sync.WaitGroup
	listener     net.Listener
	serverConfig *serverConfig
}

func NewServer(cfg *config.Config) (*Server, error) {
	serverConfig, err := newServerConfig(cfg)
	if err != nil {
		return nil, err
	}

	return &Server{Config: cfg, serverConfig: serverConfig}, nil
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	if err := s.listen(ctx); err != nil {
		return err
	}
	defer s.listener.Close()

	s.serve(ctx)

	return nil
}

func (s *Server) Shutdown() error {
	if s.listener == nil {
		return nil
	}

	s.changeStatus(StatusOnShutdown)

	return s.listener.Close()
}

func (s *Server) MonitoringServeMux() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc(s.Config.Server.ReadinessProbe, func(w http.ResponseWriter, r *http.Request) {
		if s.getStatus() == StatusReady {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	})

	mux.HandleFunc(s.Config.Server.LivenessProbe, func(w http.ResponseWriter, r *http.Request) {
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

	log.WithContextFields(ctx, log.Fields{"tcp_address": sshListener.Addr().String()}).Info("Listening for SSH connections")

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
		nconn.Close() // Close the connection when context is cancelled
	}()

	remoteAddr := nconn.RemoteAddr().String()
	ctxlog := log.WithContextFields(ctx, log.Fields{"remote_addr": remoteAddr})
	ctxlog.Debug("server: handleConn: start")

	// Prevent a panic in a single connection from taking out the whole server
	defer func() {
		if err := recover(); err != nil {
			ctxlog.Warn("panic handling session")

			metrics.SliSshdSessionsErrorsTotal.Inc()
		}
	}()

	conn := newConnection(s.Config, nconn)
	conn.handle(ctx, s.serverConfig.get(ctx), func(sconn *ssh.ServerConn, channel ssh.Channel, requests <-chan *ssh.Request) error {
		session := &session{
			cfg:                 s.Config,
			channel:             channel,
			gitlabKeyId:         sconn.Permissions.Extensions["key-id"],
			gitlabKrb5Principal: sconn.Permissions.Extensions["krb5principal"],
			remoteAddr:          remoteAddr,
			started:             time.Now(),
		}

		return session.handle(ctx, requests)
	})
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

func staticProxyPolicy(policy proxyproto.Policy) proxyproto.PolicyFunc {
	return func(_ net.Addr) (proxyproto.Policy, error) {
		return policy, nil
	}
}
