// Package sshd implements functionality related to SSH server configuration and handling
package sshd

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/authorizedcerts"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/authorizedkeys"

	"gitlab.com/gitlab-org/labkit/fips"
	"gitlab.com/gitlab-org/labkit/log"
)

type serverConfig struct {
	cfg                   *config.Config
	hostKeys              []ssh.Signer
	hostKeyToCertMap      map[string]*ssh.Certificate
	authorizedKeysClient  *authorizedkeys.Client
	authorizedCertsClient *authorizedcerts.Client
}

func parseHostKeys(keyFiles []string) []ssh.Signer {
	var hostKeys []ssh.Signer

	for _, filename := range keyFiles {
		keyRaw, err := os.ReadFile(filepath.Clean(filename))
		if err != nil {
			log.WithError(err).WithFields(log.Fields{"filename": filename}).Error("Failed to read host key")
			continue
		}
		key, err := ssh.ParsePrivateKey(keyRaw)
		if err != nil {
			log.WithError(err).WithFields(log.Fields{"filename": filename}).Error("Failed to parse host key")
			continue
		}

		hostKeys = append(hostKeys, key)
	}

	return hostKeys
}

func parseHostCerts(hostKeys []ssh.Signer, certFiles []string) map[string]*ssh.Certificate {
	keyToCertMap := map[string]*ssh.Certificate{}
	hostKeyIndex := make(map[string]int)

	for index, hostKey := range hostKeys {
		hostKeyIndex[string(hostKey.PublicKey().Marshal())] = index
	}

	for _, filename := range certFiles {
		keyRaw, err := os.ReadFile(filepath.Clean(filename))
		if err != nil {
			log.WithError(err).WithFields(log.Fields{"filename": filename}).Error("failed to read host certificate")
			continue
		}
		publicKey, _, _, _, err := ssh.ParseAuthorizedKey(keyRaw)
		if err != nil {
			log.WithError(err).WithFields(log.Fields{"filename": filename}).Error("failed to parse host certificate")
			continue
		}

		cert, ok := publicKey.(*ssh.Certificate)
		if !ok {
			log.WithFields(log.Fields{"filename": filename}).Error("failed to decode host certificate")
			continue
		}

		hostRawKey := string(cert.Key.Marshal())
		index, found := hostKeyIndex[hostRawKey]
		if found {
			keyToCertMap[hostRawKey] = cert

			certSigner, err := ssh.NewCertSigner(cert, hostKeys[index])
			if err != nil {
				log.WithError(err).WithFields(log.Fields{"filename": filename}).Error("the host certificate doesn't match the host private key")
				continue
			}

			hostKeys[index] = certSigner
		} else {
			log.WithFields(log.Fields{"filename": filename}).Errorf("no matching private key for certificate %s", filename)
		}
	}

	return keyToCertMap
}

func newServerConfig(cfg *config.Config) (*serverConfig, error) {
	authorizedKeysClient, err := authorizedkeys.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize authorized keys client: %w", err)
	}

	authorizedCertsClient, err := authorizedcerts.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize authorized certs client: %w", err)
	}

	hostKeys := parseHostKeys(cfg.Server.HostKeyFiles)
	if len(hostKeys) == 0 {
		return nil, fmt.Errorf("no host keys could be loaded, aborting")
	}

	hostKeyToCertMap := parseHostCerts(hostKeys, cfg.Server.HostCertFiles)

	return &serverConfig{
		cfg:                   cfg,
		authorizedKeysClient:  authorizedKeysClient,
		authorizedCertsClient: authorizedCertsClient,
		hostKeys:              hostKeys,
		hostKeyToCertMap:      hostKeyToCertMap,
	}, nil
}

func (s *serverConfig) handleUserKey(ctx context.Context, user string, key ssh.PublicKey) (*ssh.Permissions, error) {
	if user != s.cfg.User {
		return nil, fmt.Errorf("unknown user")
	}
	if key.Type() == ssh.KeyAlgoDSA {
		return nil, fmt.Errorf("DSA is prohibited")
	}

	res, err := s.authorizedKeysClient.GetByKey(ctx, base64.RawStdEncoding.EncodeToString(key.Marshal()))
	if err != nil {
		return nil, err
	}

	return &ssh.Permissions{
		// Record the public key used for authentication.
		Extensions: map[string]string{
			"key-id": strconv.FormatInt(res.ID, 10),
		},
	}, nil
}

func (s *serverConfig) handleUserCertificate(ctx context.Context, user string, cert *ssh.Certificate) (*ssh.Permissions, error) {
	if os.Getenv("FF_GITLAB_SHELL_SSH_CERTIFICATES") != "1" {
		return nil, fmt.Errorf("handleUserCertificate: feature is disabled")
	}

	fingerprint := ssh.FingerprintSHA256(cert.SignatureKey)

	if cert.CertType != ssh.UserCert {
		return nil, fmt.Errorf("handleUserCertificate: cert has type %d", cert.CertType)
	}

	certChecker := &ssh.CertChecker{}
	if err := certChecker.CheckCert(user, cert); err != nil {
		return nil, err
	}

	logger := log.WithContextFields(ctx,
		log.Fields{
			"ssh_user":               user,
			"public_key_fingerprint": ssh.FingerprintSHA256(cert),
			"signing_ca_fingerprint": fingerprint,
			"certificate_identity":   cert.KeyId,
		},
	)

	res, err := s.authorizedCertsClient.GetByKey(ctx, cert.KeyId, strings.TrimPrefix(fingerprint, "SHA256:"))
	if err != nil {
		logger.WithError(err).Warn("user certificate is not signed by a trusted key")

		return nil, err
	}

	logger.WithFields(
		log.Fields{
			"certificate_username":  res.Username,
			"certificate_namespace": res.Namespace,
		},
	).Info("user certificate is signed by a trusted key")

	return &ssh.Permissions{
		Extensions: map[string]string{
			"username":  res.Username,
			"namespace": res.Namespace,
		},
	}, nil
}

func (s *serverConfig) get(parentCtx context.Context) *ssh.ServerConfig {
	var gssapiWithMICConfig *ssh.GSSAPIWithMICConfig
	if s.cfg.Server.GSSAPI.Enabled {
		gssAPIServer, _ := NewGSSAPIServer(&s.cfg.Server.GSSAPI)

		if gssAPIServer != nil {
			gssapiWithMICConfig = &ssh.GSSAPIWithMICConfig{
				AllowLogin: func(conn ssh.ConnMetadata, srcName string) (*ssh.Permissions, error) {
					if conn.User() != s.cfg.User {
						return nil, fmt.Errorf("unknown user")
					}

					return &ssh.Permissions{
						// Record the Kerberos principal used for authentication.
						Extensions: map[string]string{
							"krb5principal": srcName,
						},
					}, nil
				},
				Server: gssAPIServer,
			}
		}
	}

	sshCfg := &ssh.ServerConfig{
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			ctx, cancel := context.WithTimeout(parentCtx, 10*time.Second)
			defer cancel()

			log.WithContextFields(ctx, log.Fields{"ssh_key_type": key.Type()}).Info("public key authentication")

			cert, ok := key.(*ssh.Certificate)
			if ok {
				return s.handleUserCertificate(ctx, conn.User(), cert)
			}

			return s.handleUserKey(ctx, conn.User(), key)
		},
		GSSAPIWithMICConfig: gssapiWithMICConfig,
		ServerVersion:       "SSH-2.0-GitLab-SSHD",
	}

	// This can be dropped once https://github.com/golang-fips/go/issues/316 is supported.
	// We need to constrain the list of supported algorithms for FIPS.
	algorithms := fips.SupportedAlgorithms()
	sshCfg.PublicKeyAuthAlgorithms = algorithms.PublicKeyAuths
	sshCfg.Ciphers = algorithms.Ciphers
	sshCfg.KeyExchanges = algorithms.KeyExchanges
	sshCfg.MACs = algorithms.MACs

	s.configureMACs(sshCfg)
	s.configureKeyExchanges(sshCfg)
	s.configureCiphers(sshCfg)
	s.configurePublicKeyAlgorithms(sshCfg)

	for _, key := range s.hostKeys {
		sshCfg.AddHostKey(key)
	}

	return sshCfg
}

func (s *serverConfig) configurePublicKeyAlgorithms(sshCfg *ssh.ServerConfig) {
	if len(s.cfg.Server.PublicKeyAlgorithms) > 0 {
		sshCfg.PublicKeyAuthAlgorithms = s.cfg.Server.PublicKeyAlgorithms
	}
}

func (s *serverConfig) configureCiphers(sshCfg *ssh.ServerConfig) {
	if len(s.cfg.Server.Ciphers) > 0 {
		sshCfg.Ciphers = s.cfg.Server.Ciphers
	}
}

func (s *serverConfig) configureKeyExchanges(sshCfg *ssh.ServerConfig) {
	if len(s.cfg.Server.KexAlgorithms) > 0 {
		sshCfg.KeyExchanges = s.cfg.Server.KexAlgorithms
	}
}

func (s *serverConfig) configureMACs(sshCfg *ssh.ServerConfig) {
	if len(s.cfg.Server.MACs) > 0 {
		sshCfg.MACs = s.cfg.Server.MACs
	}
}
