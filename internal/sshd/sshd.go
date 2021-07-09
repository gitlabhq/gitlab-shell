package sshd

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"strconv"
	"time"
	"sync"

	log "github.com/sirupsen/logrus"

	"github.com/pires/go-proxyproto"
	"golang.org/x/crypto/ssh"

	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet/authorizedkeys"
	"gitlab.com/gitlab-org/labkit/correlation"
)

type Server struct {
	Config *config.Config

	onShutdown bool
	wg sync.WaitGroup
	listener net.Listener
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	if err := s.listen(); err != nil {
		return err
	}
	defer s.listener.Close()

	return s.serve(ctx)
}

func (s *Server) Shutdown() error {
	if s.listener == nil {
		return nil
	}

	s.onShutdown = true

	return s.listener.Close()
}

func (s *Server) listen() error {
	sshListener, err := net.Listen("tcp", s.Config.Server.Listen)
	if err != nil {
		return fmt.Errorf("failed to listen for connection: %w", err)
	}

	if s.Config.Server.ProxyProtocol {
		sshListener = &proxyproto.Listener{Listener: sshListener}

		log.Info("Proxy protocol is enabled")
	}

	log.Infof("Listening on %v", sshListener.Addr().String())

	s.listener = sshListener

	return nil
}

func (s *Server) serve(ctx context.Context) error {
	sshCfg, err := s.initConfig(ctx)
	if err != nil {
		return err
	}

	for {
		nconn, err := s.listener.Accept()
		if err != nil {
			if s.onShutdown {
				break
			}

			log.Warnf("Failed to accept connection: %v\n", err)
			continue
		}

		s.wg.Add(1)
		go s.handleConn(ctx, sshCfg, nconn)
	}

	s.wg.Wait()

	return nil
}

func (s *Server) initConfig(ctx context.Context) (*ssh.ServerConfig, error) {
	authorizedKeysClient, err := authorizedkeys.NewClient(s.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize GitLab client: %w", err)
	}

	sshCfg := &ssh.ServerConfig{
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if conn.User() != s.Config.User {
				return nil, errors.New("unknown user")
			}
			if key.Type() == ssh.KeyAlgoDSA {
				return nil, errors.New("DSA is prohibited")
			}
			ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			res, err := authorizedKeysClient.GetByKey(ctx, base64.RawStdEncoding.EncodeToString(key.Marshal()))
			if err != nil {
				return nil, err
			}

			return &ssh.Permissions{
				// Record the public key used for authentication.
				Extensions: map[string]string{
					"key-id": strconv.FormatInt(res.Id, 10),
				},
			}, nil
		},
	}

	var loadedHostKeys uint
	for _, filename := range s.Config.Server.HostKeyFiles {
		keyRaw, err := ioutil.ReadFile(filename)
		if err != nil {
			log.Warnf("Failed to read host key %v: %v", filename, err)
			continue
		}
		key, err := ssh.ParsePrivateKey(keyRaw)
		if err != nil {
			log.Warnf("Failed to parse host key %v: %v", filename, err)
			continue
		}
		loadedHostKeys++
		sshCfg.AddHostKey(key)
	}
	if loadedHostKeys == 0 {
		return nil, fmt.Errorf("No host keys could be loaded, aborting")
	}

	return sshCfg, nil
}


func (s *Server) handleConn(ctx context.Context, sshCfg *ssh.ServerConfig, nconn net.Conn) {
	remoteAddr := nconn.RemoteAddr().String()

	defer s.wg.Done()
	defer nconn.Close()

	// Prevent a panic in a single connection from taking out the whole server
	defer func() {
		if err := recover(); err != nil {
			log.Warnf("panic handling connection from %s: recovered: %#+v", remoteAddr, err)
		}
	}()

	ctx, cancel := context.WithCancel(correlation.ContextWithCorrelation(ctx, correlation.SafeRandomID()))
	defer cancel()

	sconn, chans, reqs, err := ssh.NewServerConn(nconn, sshCfg)
	if err != nil {
		log.Infof("Failed to initialize SSH connection: %v", err)
		return
	}

	go ssh.DiscardRequests(reqs)

	conn := newConnection(s.Config.Server.ConcurrentSessionsLimit, remoteAddr)
	conn.handle(ctx, chans, func(ctx context.Context, channel ssh.Channel, requests <-chan *ssh.Request) {
		session := &session{
			cfg:         s.Config,
			channel:     channel,
			gitlabKeyId: sconn.Permissions.Extensions["key-id"],
			remoteAddr:  remoteAddr,
		}

		session.handle(ctx, requests)
	})
}
