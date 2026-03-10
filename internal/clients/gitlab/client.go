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
	"path/filepath"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"gitlab.com/gitlab-org/labkit/v2/httpclient"
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
	// Secret is the HS256 JWT signing secret.
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
	transport, host, err := buildTransport(cfg)
	if err != nil {
		return nil, err
	}

	timeout := time.Duration(cfg.ReadTimeoutSeconds) * time.Second
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

func (c *Client) do(ctx context.Context, method, path string, data any) (*http.Response, error) {
	url := strings.TrimSuffix(c.host, "/") + normalizePath(path)

	var bodyReader io.Reader
	if data != nil {
		encoded, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("marshaling request body: %w", err)
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

func (c *Client) jwtToken() (string, error) {
	claims := jwt.RegisteredClaims{
		Issuer:    jwtIssuer,
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(jwtTTL)),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(strings.TrimSpace(c.secret)))
}

func normalizePath(path string) string {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if !strings.HasPrefix(path, internalAPIPath) {
		path = internalAPIPath + path
	}
	return path
}

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

func buildHTTPSTransport(cfg *Config) (http.RoundTripper, error) {
	certPool, err := x509.SystemCertPool()
	if err != nil {
		certPool = x509.NewCertPool()
	}

	if cfg.CaFile != "" {
		if cert, readErr := os.ReadFile(filepath.Clean(cfg.CaFile)); readErr == nil {
			certPool.AppendCertsFromPEM(cert)
		}
	}

	if cfg.CaPath != "" {
		entries, _ := os.ReadDir(cfg.CaPath)
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if cert, readErr := os.ReadFile(filepath.Join(cfg.CaPath, entry.Name())); readErr == nil {
				certPool.AppendCertsFromPEM(cert)
			}
		}
	}

	return &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs:    certPool,
			MinVersion: tls.VersionTLS12,
		},
	}, nil
}
