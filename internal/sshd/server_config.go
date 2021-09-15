package sshd

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"time"

	"golang.org/x/crypto/ssh"

	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet/authorizedkeys"

	"gitlab.com/gitlab-org/labkit/log"
)

type serverConfig struct {
	cfg                  *config.Config
	hostKeys             []ssh.Signer
	authorizedKeysClient *authorizedkeys.Client
}

func newServerConfig(cfg *config.Config) (*serverConfig, error) {
	authorizedKeysClient, err := authorizedkeys.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize GitLab client: %w", err)
	}

	var hostKeys []ssh.Signer
	for _, filename := range cfg.Server.HostKeyFiles {
		keyRaw, err := os.ReadFile(filename)
		if err != nil {
			log.WithError(err).WithFields(log.Fields{"filename": filename}).Warn("Failed to read host key")
			continue
		}
		key, err := ssh.ParsePrivateKey(keyRaw)
		if err != nil {
			log.WithError(err).WithFields(log.Fields{"filename": filename}).Warn("Failed to parse host key")
			continue
		}

		hostKeys = append(hostKeys, key)
	}
	if len(hostKeys) == 0 {
		return nil, fmt.Errorf("No host keys could be loaded, aborting")
	}

	return &serverConfig{cfg: cfg, authorizedKeysClient: authorizedKeysClient, hostKeys: hostKeys}, nil
}

func (s *serverConfig) getAuthKey(ctx context.Context, user string, key ssh.PublicKey) (*authorizedkeys.Response, error) {
	if user != s.cfg.User {
		return nil, fmt.Errorf("unknown user")
	}
	if key.Type() == ssh.KeyAlgoDSA {
		return nil, fmt.Errorf("DSA is prohibited")
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	res, err := s.authorizedKeysClient.GetByKey(ctx, base64.RawStdEncoding.EncodeToString(key.Marshal()))
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (s *serverConfig) get(ctx context.Context) *ssh.ServerConfig {
	sshCfg := &ssh.ServerConfig{
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			res, err := s.getAuthKey(ctx, conn.User(), key)
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

	for _, key := range s.hostKeys {
		sshCfg.AddHostKey(key)
	}

	return sshCfg
}
