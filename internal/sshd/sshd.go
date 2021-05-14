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

	log "github.com/sirupsen/logrus"

	"github.com/pires/go-proxyproto"
	"golang.org/x/crypto/ssh"

	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet/authorizedkeys"
	"gitlab.com/gitlab-org/labkit/correlation"
)

func Run(ctx context.Context, cfg *config.Config) error {
	authorizedKeysClient, err := authorizedkeys.NewClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize GitLab client: %w", err)
	}

	sshListener, err := net.Listen("tcp", cfg.Server.Listen)
	if err != nil {
		return fmt.Errorf("failed to listen for connection: %w", err)
	}
	if cfg.Server.ProxyProtocol {
		sshListener = &proxyproto.Listener{Listener: sshListener}

		log.Info("Proxy protocol is enabled")
	}
	defer sshListener.Close()

	log.Infof("Listening on %v", sshListener.Addr().String())

	sshCfg := &ssh.ServerConfig{
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if conn.User() != cfg.User {
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
	for _, filename := range cfg.Server.HostKeyFiles {
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
		return fmt.Errorf("No host keys could be loaded, aborting")
	}

	for {
		nconn, err := sshListener.Accept()
		if err != nil {
			log.Warnf("Failed to accept connection: %v\n", err)
			continue
		}

		go handleConn(ctx, cfg, sshCfg, nconn)
	}
}

func handleConn(ctx context.Context, cfg *config.Config, sshCfg *ssh.ServerConfig, nconn net.Conn) {
	remoteAddr := nconn.RemoteAddr().String()

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

	conn := newConnection(cfg.Server.ConcurrentSessionsLimit, remoteAddr)
	conn.handle(ctx, chans, func(ctx context.Context, channel ssh.Channel, requests <-chan *ssh.Request) {
		session := &session{
			cfg:         cfg,
			channel:     channel,
			gitlabKeyId: sconn.Permissions.Extensions["key-id"],
			remoteAddr:  remoteAddr,
		}

		session.handle(ctx, requests)
	})
}
