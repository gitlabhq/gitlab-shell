// Package gitlab provides an HTTP client for the GitLab internal API.
// It is the replacement for the client and gitlabnet packages and is being
// introduced incrementally as part of the consolidation described in
// https://gitlab.com/gitlab-org/gitlab-shell/-/issues/834.
package gitlab

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"gitlab.com/gitlab-org/labkit/correlation"
	"gitlab.com/gitlab-org/labkit/v2/httpclient"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/metrics"
)

const (
	internalAPIPath     = "/api/v4/internal"
	apiSecretHeaderName = "Gitlab-Shell-Api-Request" // #nosec G101
	defaultUserAgent    = "GitLab-Shell"
	jwtTTL              = time.Minute
	jwtIssuer           = "gitlab-shell"
	defaultReadTimeout  = 300 * time.Second

	socketBaseURL      = "http://unix"
	unixSocketProtocol = "http+unix://"
	httpProtocol       = "http://"
	httpsProtocol      = "https://"
)

// Config holds the configuration for the GitLab internal API client.
type Config struct {
	// GitlabURL is the base URL of the GitLab instance.
	GitlabURL string
	// RelativeURLRoot is an optional relative URL root prefix (for Unix socket URLs).
	RelativeURLRoot string
	// User is the HTTP basic auth username.
	User string
	// Password is the HTTP basic auth password.
	Password string
	// Secret is the HS256 JWT signing secret. Must not be empty.
	Secret string
	// CaFile is the path to a custom CA certificate file.
	CaFile string
	// CaPath is the path to a directory of custom CA certificate files.
	CaPath string
	// ReadTimeoutSeconds is the HTTP read timeout. Defaults to 300s when zero.
	ReadTimeoutSeconds uint64
}

// Client is an HTTP client for the GitLab internal API.
type Client struct {
	inner    *httpclient.Client
	host     string
	user     string
	password string
	secret   string
}

// New creates a new Client from the given Config.
func New(cfg *Config) (*Client, error) {
	if cfg == nil {
		return nil, errors.New("config must not be nil")
	}

	if strings.TrimSpace(cfg.Secret) == "" {
		return nil, errors.New("secret must not be empty")
	}

	transport, host, err := buildTransport(cfg)
	if err != nil {
		return nil, err
	}

	// Layer cross-cutting concerns on top of the base transport, innermost first:
	//   1. forwardedIPTransport — propagates X-Forwarded-For from context so that
	//      GitLab can log the original client IP for SSH-over-HTTP connections.
	//   2. metrics.NewRoundTripper — instruments every request with Prometheus
	//      counters/histograms, matching the old config.HTTPClient() behavior.
	//   3. correlation.NewInstrumentedRoundTripper — injects the correlation ID
	//      from context as the X-Request-Id header, matching the old transport chain.
	transport = &forwardedIPTransport{next: transport}
	transport = metrics.NewRoundTripper(transport)
	transport = correlation.NewInstrumentedRoundTripper(transport)

	timeout := time.Duration(cfg.ReadTimeoutSeconds) * time.Second // #nosec G115
	if cfg.ReadTimeoutSeconds == 0 {
		timeout = defaultReadTimeout
	}

	inner := httpclient.NewWithConfig(&httpclient.Config{
		Transport: transport,
		Timeout:   timeout,
	})

	return &Client{
		inner:    inner,
		host:     host,
		user:     cfg.User,
		password: cfg.Password,
		secret:   cfg.Secret,
	}, nil
}

// Get makes a GET request to the given internal API path.
func (c *Client) Get(ctx context.Context, path string) (*http.Response, error) {
	return c.do(ctx, http.MethodGet, path, nil)
}

// Post makes a POST request to the given internal API path with a JSON body.
func (c *Client) Post(ctx context.Context, path string, body any) (*http.Response, error) {
	return c.do(ctx, http.MethodPost, path, body)
}

// do is the single request path for all outbound calls. It exists to keep
// Get/Post thin and ensure that header injection (JWT, basic auth, User-Agent)
// is applied consistently regardless of the HTTP method. During the migration
// from client.GitlabNetClient, additional methods (PUT, DELETE, …) can be
// added here without duplicating the auth logic.
func (c *Client) do(ctx context.Context, method, path string, data any) (*http.Response, error) {
	normalized, err := normalizePath(path)
	if err != nil {
		return nil, err
	}
	url := strings.TrimSuffix(c.host, "/") + normalized

	var bodyReader io.Reader
	if data != nil {
		encoded, marshalErr := json.Marshal(data)
		if marshalErr != nil {
			return nil, fmt.Errorf("marshaling request body: %w", marshalErr)
		}
		bodyReader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	if err := c.setHeaders(req); err != nil {
		return nil, err
	}

	return c.inner.Do(req)
}

// setHeaders stamps every outbound request with the three auth/identity
// signals that the GitLab internal API requires:
//
//   - Basic auth — used by some GitLab deployments that sit behind an
//     nginx auth_basic gate; mirrored from HTTPSettingsConfig.User/Password
//     in the existing config package. Both username and password must be
//     non-empty, matching the behavior of the old client.GitlabNetClient.
//   - JWT bearer token — the primary machine-to-machine secret; the Rails
//     side validates the HS256 signature and rejects requests whose token has
//     expired or was signed with the wrong key.
//   - User-Agent — helps GitLab ops distinguish gitlab-shell traffic in
//     access logs; kept identical to the value used by client.GitlabNetClient
//     so log-based dashboards do not need updating during the migration.
func (c *Client) setHeaders(req *http.Request) error {
	if c.user != "" && c.password != "" {
		req.SetBasicAuth(c.user, c.password)
	}

	token, err := c.jwtToken()
	if err != nil {
		return err
	}

	req.Header.Set(apiSecretHeaderName, token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", defaultUserAgent)

	return nil
}

// jwtToken mints a short-lived HS256 token signed with the shared secret.
// A fresh token is generated per request because the TTL is only one minute;
// reusing a cached token across requests risks sending an expired credential
// if the caller batches requests or retries after a delay. This matches the
// behavior of client.GitlabNetClient.DoRequest.
func (c *Client) jwtToken() (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Issuer:    jwtIssuer,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(jwtTTL)),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(strings.TrimSpace(c.secret)))
}

// normalizePath ensures every path is rooted under /api/v4/internal. This
// mirrors the logic in client.GitlabNetClient so that callers migrated to
// this package can pass the same short paths (e.g. "/check", "lfs/objects")
// without any changes at the call site.
//
// path.Clean is applied after prefixing to collapse any traversal segments
// (e.g. "/../") and repeated slashes. An error is returned if the cleaned
// result no longer starts with /api/v4/internal, which would indicate a
// traversal attempt escaping the internal API prefix.
func normalizePath(p string) (string, error) {
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if !strings.HasPrefix(p, internalAPIPath) {
		p = internalAPIPath + p
	}
	cleaned := path.Clean(p)
	if !strings.HasPrefix(cleaned, internalAPIPath) {
		return "", fmt.Errorf("path %q escapes the internal API prefix", p)
	}
	return cleaned, nil
}

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
		req.Header.Add("X-Forwarded-For", ip)
	}
	return t.next.RoundTrip(req)
}
