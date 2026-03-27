// Package sshd implements functionality related to SSH server configuration and handling
package sshd

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/authorizedcerts"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/authorizedkeys"

	"gitlab.com/gitlab-org/labkit/fips"
	"gitlab.com/gitlab-org/labkit/v2/log"
)

type serverConfig struct {
	cfg                   *config.Config
	hostKeys              []ssh.Signer
	hostKeyToCertMap      map[string]*ssh.Certificate
	trustedUserCAKeySet   map[string]struct{}
	authorizedKeysClient  *authorizedkeys.Client
	authorizedCertsClient *authorizedcerts.Client
}

func parseHostKeys(keyFiles []string) []ssh.Signer {
	var hostKeys []ssh.Signer

	for _, filename := range keyFiles {
		keyRaw, err := os.ReadFile(filepath.Clean(filename))
		if err != nil {
			slog.Error("Failed to read host key", slog.String("filename", filename), log.ErrorMessage(err.Error()))
			continue
		}
		key, err := ssh.ParsePrivateKey(keyRaw)
		if err != nil {
			slog.Error("Failed to parse host key", slog.String("filename", filename), log.ErrorMessage(err.Error()))
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
		ctx := context.Background()
		ctx = log.WithFields(ctx, slog.String("filename", filename))
		if err != nil {
			slog.ErrorContext(ctx, "failed to read host certificate", log.ErrorMessage(err.Error()))
			continue
		}
		publicKey, _, _, _, err := ssh.ParseAuthorizedKey(keyRaw)
		if err != nil {
			slog.ErrorContext(ctx, "failed to parse host certificate", log.ErrorMessage(err.Error()))
			continue
		}

		cert, ok := publicKey.(*ssh.Certificate)
		if !ok {
			slog.ErrorContext(ctx, "failed to decode host certificate")
			continue
		}

		hostRawKey := string(cert.Key.Marshal())
		index, found := hostKeyIndex[hostRawKey]
		if found {
			keyToCertMap[hostRawKey] = cert

			certSigner, err := ssh.NewCertSigner(cert, hostKeys[index])
			if err != nil {
				slog.ErrorContext(ctx, "the host certificate doesn't match the host private key", log.ErrorMessage(err.Error()))
				continue
			}

			hostKeys[index] = certSigner
		} else {
			slog.ErrorContext(ctx, "no matching private key for certificate")
		}
	}

	return keyToCertMap
}

// parseTrustedUserCAKeys loads trusted user CA public key files.
// Unlike parseHostKeys, this fails on any error because trusted CA keys are a
// security trust boundary: a partially loaded set could silently authenticate
// the wrong users or fail to authenticate expected users.
func parseTrustedUserCAKeys(caKeyFiles []string) (map[string]struct{}, error) {
	result := make(map[string]struct{})

	for _, filename := range caKeyFiles {
		keyRaw, err := os.ReadFile(filepath.Clean(filename))
		if err != nil {
			return nil, fmt.Errorf("failed to read trusted user CA key file %q: %w", filename, err)
		}

		rest := keyRaw
		keysFromFile := 0
		for len(rest) > 0 {
			publicKey, _, _, remaining, err := ssh.ParseAuthorizedKey(rest)
			if err != nil {
				return nil, fmt.Errorf("failed to parse trusted user CA key in file %q after %d valid key(s): %w", filename, keysFromFile, err)
			}
			result[string(publicKey.Marshal())] = struct{}{}
			keysFromFile++
			rest = remaining
		}
	}

	return result, nil
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

	trustedUserCAKeySet, err := parseTrustedUserCAKeys(cfg.Server.TrustedUserCAKeys)
	if err != nil {
		return nil, fmt.Errorf("failed to load trusted user CA keys: %w", err)
	}
	if len(cfg.Server.TrustedUserCAKeys) > 0 && len(trustedUserCAKeySet) == 0 {
		return nil, fmt.Errorf("trusted_user_ca_keys configured but no valid CA keys were loaded, aborting")
	}
	if len(trustedUserCAKeySet) > 0 {
		slog.Info("Loaded trusted user CA keys for instance-level SSH certificates",
			slog.Int("count", len(trustedUserCAKeySet)))
	}

	return &serverConfig{
		cfg:                   cfg,
		authorizedKeysClient:  authorizedKeysClient,
		authorizedCertsClient: authorizedCertsClient,
		hostKeys:              hostKeys,
		hostKeyToCertMap:      hostKeyToCertMap,
		trustedUserCAKeySet:   trustedUserCAKeySet,
	}, nil
}

func (s *serverConfig) isLocallyTrustedCA(signingKey ssh.PublicKey) bool {
	_, ok := s.trustedUserCAKeySet[string(signingKey.Marshal())]
	return ok
}

var (
	validKeyIDPattern       = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,253}[a-zA-Z0-9]$`)
	consecutiveSpecialChars = regexp.MustCompile(`[._-]{2,}`)
)

// validateKeyID checks that the certificate KeyId conforms to GitLab's username
// rules, since it is used directly as the username in API calls and logging.
func validateKeyID(keyID string) error {
	if keyID == "" {
		return fmt.Errorf("certificate has empty KeyId")
	}
	if len(keyID) < 2 || len(keyID) > 255 {
		return fmt.Errorf("certificate KeyId length %d is outside valid range [2, 255]", len(keyID))
	}
	if !validKeyIDPattern.MatchString(keyID) {
		return fmt.Errorf("certificate KeyId does not match GitLab username format")
	}
	if consecutiveSpecialChars.MatchString(keyID) {
		return fmt.Errorf("certificate KeyId contains consecutive special characters")
	}
	return nil
}

func (s *serverConfig) handleUserKey(ctx context.Context, user string, key ssh.PublicKey) (*ssh.Permissions, error) {
	if user != s.cfg.User {
		return nil, fmt.Errorf("unknown user")
	}
	//nolint:staticcheck // SA1019: Intentionally checking for deprecated DSA to reject it
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

func (s *serverConfig) handleUserCertificate(ctx context.Context, user string, cert *ssh.Certificate) (*ssh.Permissions, error) { //nolint:funlen
	fingerprint := ssh.FingerprintSHA256(cert.SignatureKey)

	// Enrich context early so all rejection paths include audit-relevant fields.
	ctx = log.WithFields(ctx,
		slog.String("ssh_user", user),
		slog.String("public_key_fingerprint", ssh.FingerprintSHA256(cert)),
		slog.String("signing_ca_fingerprint", fingerprint),
		slog.String("certificate_identity", cert.KeyId),
	)

	if cert.CertType != ssh.UserCert {
		slog.WarnContext(ctx, "certificate rejected: not a user certificate",
			slog.Int("cert_type", int(cert.CertType)))
		return nil, fmt.Errorf("handleUserCertificate: cert has type %d", cert.CertType)
	}

	certChecker := &ssh.CertChecker{}
	if err := certChecker.CheckCert(user, cert); err != nil {
		slog.WarnContext(ctx, "certificate rejected: validity check failed",
			log.ErrorMessage(err.Error()))
		return nil, err
	}

	if s.isLocallyTrustedCA(cert.SignatureKey) {
		if err := validateKeyID(cert.KeyId); err != nil {
			slog.WarnContext(ctx, "instance-level certificate rejected: invalid KeyId",
				log.ErrorMessage(err.Error()))
			return nil, fmt.Errorf("handleUserCertificate: %w", err)
		}

		ctx = log.WithFields(ctx, slog.String("certificate_username", cert.KeyId))
		slog.InfoContext(ctx, "user certificate is signed by a locally trusted CA (instance-level)")

		// No namespace key = instance-wide access (no namespace restriction)
		return &ssh.Permissions{
			Extensions: map[string]string{
				"username": cert.KeyId,
			},
		}, nil
	}

	// Fall back to group-level certificate check via Rails API
	if os.Getenv("FF_GITLAB_SHELL_SSH_CERTIFICATES") != "1" {
		return nil, fmt.Errorf("handleUserCertificate: feature is disabled")
	}

	res, err := s.authorizedCertsClient.GetByKey(ctx, cert.KeyId, strings.TrimPrefix(fingerprint, "SHA256:"))
	if err != nil {
		slog.WarnContext(ctx, "user certificate is not signed by a trusted key", log.ErrorMessage(err.Error()))
		return nil, err
	}

	ctx = log.WithFields(ctx,
		slog.String("certificate_username", res.Username),
		slog.String("certificate_namespace", res.Namespace),
	)

	slog.InfoContext(ctx, "user certificate is signed by a trusted key (group-level)")

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
			slog.InfoContext(ctx, "public key authentication", slog.String("ssh_key_type", key.Type()))

			cert, ok := key.(*ssh.Certificate)
			if ok {
				return s.handleUserCertificate(ctx, conn.User(), cert)
			}

			return s.handleUserKey(ctx, conn.User(), key)
		},
		GSSAPIWithMICConfig: gssapiWithMICConfig,
		ServerVersion:       "SSH-2.0-GitLab-SSHD",
	}

	// Only set this for FIPS because by default to preserve backwards compatibility
	// for previous versions that support both secure and insecure defaults.
	if fips.Enabled() {
		// This can be dropped once https://github.com/golang-fips/go/issues/316 is supported.
		// We need to constrain the list of supported algorithms for FIPS because
		// ED25519 algorithms cause gitlab-sshd to panic.
		//
		// Right now we use fips.DefaultAlgorithms() instead of fips.SupportedAlgorithms()
		// to preserve backwards compatibility with clients that are not configured properly.
		// fips.DefaultAlgorithms() still allows ssh-rsa and ssh-dss. Admins can lock down
		// these algorithms by setting `public_key_algorithms`.
		algorithms := fips.DefaultAlgorithms()
		sshCfg.PublicKeyAuthAlgorithms = algorithms.PublicKeyAuths
		sshCfg.Ciphers = algorithms.Ciphers
		sshCfg.KeyExchanges = algorithms.KeyExchanges
		sshCfg.MACs = algorithms.MACs
	}

	s.configureMACs(sshCfg)
	s.configureKeyExchanges(sshCfg)
	s.configureCiphers(sshCfg)
	s.configurePublicKeyAlgorithms(sshCfg)

	for _, key := range s.hostKeys {
		sshCfg.AddHostKey(key)
	}

	sshCfg.SetDefaults()

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
