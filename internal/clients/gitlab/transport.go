package gitlab

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
)

const (
	socketBaseURL      = "http://unix"
	unixSocketProtocol = "http+unix://"
	httpProtocol       = "http://"
	httpsProtocol      = "https://"
)

// buildTransport selects the appropriate base http.RoundTripper for the
// configured URL scheme and returns it alongside the resolved host string
// that all request URLs will be prefixed with. The three schemes below
// replicate the behavior of client.NewHTTPClientWithOpts, which is the
// function this package is intended to replace:
//
//   - http+unix:// — GitLab Workhorse / Rails communicate over a Unix domain
//     socket in most single-node deployments. The standard net/http stack does
//     not understand this scheme, so the DialContext is overridden to open a
//     unix socket, and the URL is rewritten to http://unix/… so that the
//     net/http request machinery produces valid Host headers.
//   - http:// — plain TCP; no additional transport configuration required.
//   - https:// — TLS with optional custom CA bundle; see buildHTTPSTransport.
//
// The returned transport is passed to httpclient.NewWithConfig as
// Config.Transport, which layers LabKit's OTel tracing and structured logging
// on top of it.
func buildTransport(cfg *Config) (http.RoundTripper, string, error) {
	switch {
	case strings.HasPrefix(cfg.GitlabURL, unixSocketProtocol):
		t, host := buildSocketTransport(cfg.GitlabURL, cfg.RelativeURLRoot)
		return t, host, nil
	case strings.HasPrefix(cfg.GitlabURL, httpProtocol):
		return &http.Transport{}, cfg.GitlabURL, nil
	case strings.HasPrefix(cfg.GitlabURL, httpsProtocol):
		t, err := buildHTTPSTransport(cfg)
		if err != nil {
			return nil, "", err
		}
		return t, cfg.GitlabURL, nil
	default:
		return nil, "", errors.New("unknown GitLab URL prefix")
	}
}

// buildSocketTransport creates a transport that dials over a Unix domain
// socket. Because net/http requires an HTTP host in request URLs, the host
// is set to the synthetic value "http://unix" (with an optional relative URL
// root appended). The DialContext ignores the network/address arguments
// supplied by net/http and always opens the socket path extracted from the
// original http+unix:// URL, which is the same approach used by the existing
// client package.
func buildSocketTransport(gitlabURL, relativeURLRoot string) (http.RoundTripper, string) {
	socketPath := strings.TrimPrefix(gitlabURL, unixSocketProtocol)
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
		},
	}
	host := socketBaseURL
	if r := strings.Trim(relativeURLRoot, "/"); r != "" {
		host = host + "/" + r
	}
	return transport, host
}

// buildHTTPSTransport constructs a TLS-enabled transport. GitLab installations
// that use a privately-signed certificate (common in self-managed deployments)
// pass either a single CA file or a directory of CA files via the config YAML.
// Both are loaded into the cert pool so that TLS handshakes succeed without
// disabling certificate verification. The minimum TLS version is pinned to
// 1.2 to match the existing client package and GitLab's own TLS policy.
//
// An error is returned if a specified CA file cannot be read or if it contains
// no valid PEM certificates, making misconfigured TLS explicit at startup
// rather than failing silently at connection time.
func buildHTTPSTransport(cfg *Config) (http.RoundTripper, error) {
	certPool, err := x509.SystemCertPool()
	if err != nil {
		certPool = x509.NewCertPool()
	}

	if err := appendCaFile(certPool, cfg.CaFile); err != nil {
		return nil, err
	}
	if err := appendCaDir(certPool, cfg.CaPath); err != nil {
		return nil, err
	}

	return &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs:    certPool,
			MinVersion: tls.VersionTLS12,
		},
	}, nil
}

// appendCaFile loads a single PEM CA certificate file into pool.
func appendCaFile(pool *x509.CertPool, caFile string) error {
	if caFile == "" {
		return nil
	}
	cert, err := os.ReadFile(filepath.Clean(caFile))
	if err != nil {
		return fmt.Errorf("reading CA file %q: %w", caFile, err)
	}
	if !pool.AppendCertsFromPEM(cert) {
		return fmt.Errorf("CA file %q contains no valid PEM certificates", caFile)
	}
	return nil
}

// appendCaDir loads every certificate file found in caPath into pool.
// Non-certificate files (e.g. README, .keep) are skipped before reading to
// avoid unnecessary I/O. Only .pem and .crt files are considered; files with
// those extensions that contain no valid PEM certificates are treated as errors.
func appendCaDir(pool *x509.CertPool, caPath string) error {
	if caPath == "" {
		return nil
	}
	entries, err := os.ReadDir(caPath)
	if err != nil {
		return fmt.Errorf("reading CA directory %q: %w", caPath, err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !isCertificateFile(entry.Name()) {
			continue
		}
		if err := appendCaDirEntry(pool, caPath, entry.Name()); err != nil {
			return err
		}
	}
	return nil
}

func isCertificateFile(name string) bool {
	return strings.HasSuffix(name, ".pem") || strings.HasSuffix(name, ".crt")
}

func appendCaDirEntry(pool *x509.CertPool, dir, name string) error {
	p := filepath.Join(dir, name)
	cert, err := os.ReadFile(p) // #nosec G304
	if err != nil {
		return fmt.Errorf("reading CA file %q: %w", p, err)
	}
	if !pool.AppendCertsFromPEM(cert) {
		return fmt.Errorf("CA file %q contains no valid PEM certificates", p)
	}
	return nil
}

// forwardedIPTransport propagates the original client IP address as the
// X-Forwarded-For header on every outbound request. The IP is read from the
// request context using client.OriginalRemoteIPContextKey, which is set by the
// SSH server when it accepts a connection. This matches the behavior of the old
// client.transport.RoundTrip so that GitLab access logs continue to show the
// real client IP rather than the gitlab-shell host address.
type forwardedIPTransport struct {
	next http.RoundTripper
}

func (t *forwardedIPTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if ip, ok := req.Context().Value(client.OriginalRemoteIPContextKey{}).(string); ok && ip != "" {
		req = req.Clone(req.Context())
		req.Header.Set("X-Forwarded-For", ip)
	}
	return t.next.RoundTrip(req)
}
