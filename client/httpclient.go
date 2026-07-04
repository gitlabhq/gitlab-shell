// Package client provides functionality for interacting with HTTP clients
package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
)

const (
	socketBaseURL             = "http://unix"
	unixSocketProtocol        = "http+unix://"
	httpProtocol              = "http://"
	httpsProtocol             = "https://"
	defaultReadTimeoutSeconds = 300
	defaultRetryWaitMinimum   = time.Second
	defaultRetryWaitMaximum   = 15 * time.Second
	defaultRetryMax           = 2
)

// ErrCafileNotFound indicates that the specified CA file was not found
var ErrCafileNotFound = errors.New("cafile not found")

// HTTPClient provides an HTTP client with retry capabilities.
// Fields other than Host must be safe to share across shallow copies,
// because GitlabNetClient.WithHost creates a copy with a different Host
// while sharing the same RetryableHTTP transport.
type HTTPClient struct {
	RetryableHTTP *retryablehttp.Client
	Host          string
}

type httpClientCfg struct {
	keyPath, certPath          string
	caFile, caPath             string
	retryWaitMin, retryWaitMax time.Duration
	retryMax                   int
}

func (hcc httpClientCfg) HaveCertAndKey() bool { return hcc.keyPath != "" && hcc.certPath != "" }

// HTTPClientOpt provides options for configuring an HttpClient
type HTTPClientOpt func(*httpClientCfg)

// WithClientCert will configure the HttpClient to provide client certificates
// when connecting to a server.
func WithClientCert(certPath, keyPath string) HTTPClientOpt {
	return func(hcc *httpClientCfg) {
		hcc.keyPath = keyPath
		hcc.certPath = certPath
	}
}

// WithHTTPRetryOpts configures HTTP retry options for the HttpClient
func WithHTTPRetryOpts(waitMin, waitMax time.Duration, maxAttempts int) HTTPClientOpt {
	return func(hcc *httpClientCfg) {
		hcc.retryWaitMin = waitMin
		hcc.retryWaitMax = waitMax
		hcc.retryMax = maxAttempts
	}
}

func validateCaFile(filename string) error {
	if filename == "" {
		return nil
	}

	if _, err := os.Stat(filename); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("cannot find cafile '%s': %w", filename, ErrCafileNotFound)
		}

		return err
	}

	return nil
}

// NewHTTPClientWithOpts builds an HTTP client using the provided options
func NewHTTPClientWithOpts(gitlabURL, gitlabRelativeURLRoot, caFile, caPath string, readTimeoutSeconds uint64, opts []HTTPClientOpt) (*HTTPClient, error) {
	hcc := &httpClientCfg{
		caFile:       caFile,
		caPath:       caPath,
		retryWaitMin: defaultRetryWaitMinimum,
		retryWaitMax: defaultRetryWaitMaximum,
		retryMax:     defaultRetryMax,
	}

	for _, opt := range opts {
		opt(hcc)
	}

	var transport *http.Transport
	var host string
	var err error
	switch {
	case strings.HasPrefix(gitlabURL, unixSocketProtocol):
		transport, host = buildSocketTransport(gitlabURL, gitlabRelativeURLRoot)
	case strings.HasPrefix(gitlabURL, httpProtocol):
		transport, host = buildHTTPTransport(gitlabURL)
	case strings.HasPrefix(gitlabURL, httpsProtocol):
		err = validateCaFile(caFile)
		if err != nil {
			return nil, err
		}
		transport, host, err = buildHTTPSTransport(*hcc, gitlabURL)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("unknown GitLab URL prefix")
	}

	c := retryablehttp.NewClient()
	c.RetryMax = hcc.retryMax
	c.RetryWaitMax = hcc.retryWaitMax
	c.RetryWaitMin = hcc.retryWaitMin
	c.Logger = nil
	c.HTTPClient.Transport = NewTransport(transport)
	c.HTTPClient.Timeout = readTimeout(readTimeoutSeconds)

	// The internal API (/api/v4/internal/*) must never be redirected. Go's
	// default redirect policy follows 3xx responses and, on a 301/302/303,
	// downgrades a POST to a GET and drops the body. That silently misroutes
	// internal API requests (e.g. to a public host that bounces http->https),
	// turning them into method-downgraded GETs that 404. Refuse to follow
	// redirects so they surface as errors instead; parseError reports any
	// status matching IsFollowedRedirect as a failure.
	c.HTTPClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}

	client := &HTTPClient{RetryableHTTP: c, Host: host}

	return client, nil
}

// IsFollowedRedirect reports whether code is one of the 3xx statuses that Go's
// http.Client would follow, i.e. the ones a CheckRedirect hook intercepts. A
// followed redirect downgrades a POST to a GET on 301/302/303 and drops the
// body, so internal API clients refuse them and treat them as errors.
//
// 300 Multiple Choices and 304/305/306 are deliberately excluded: Go does not
// follow them, and the GitLab internal API uses 300 for custom actions (e.g.
// Geo) whose body must be parsed normally.
func IsFollowedRedirect(code int) bool {
	switch code {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return true
	default:
		return false
	}
}

// IsSystemErrorStatus reports whether an HTTP status from the internal API
// unambiguously indicates a gitlab-shell/infrastructure failure. Transport-layer
// logging and APIError.System both use this status-only classification.
//
//   - Followed redirects (301/302/303/307/308): misroute → system.
//   - 400 Bad Request: shell-facing internal API endpoints map policy outcomes
//     to 401/403/404/422; a 400 indicates a malformed request from
//     gitlab-shell, such as grape parameter-validation failures or bad_request!
//     on a corrupt gitaly_client_context_bin → system.
//   - 5xx: server-side failure → system.
//
// Other 4xx responses (401/403/404/422/429) are expected policy responses.
// parseError reuses this function so that error-level logging and the error-SLI
// (APIError.System) classification agree.
func IsSystemErrorStatus(code int) bool {
	return IsFollowedRedirect(code) || code == http.StatusBadRequest || code >= 500
}

func buildSocketTransport(gitlabURL, gitlabRelativeURLRoot string) (*http.Transport, string) {
	socketPath := strings.TrimPrefix(gitlabURL, unixSocketProtocol)

	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			dialer := net.Dialer{}
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}

	host := socketBaseURL
	gitlabRelativeURLRoot = strings.Trim(gitlabRelativeURLRoot, "/")
	if gitlabRelativeURLRoot != "" {
		host = host + "/" + gitlabRelativeURLRoot
	}

	return transport, host
}

func buildHTTPSTransport(hcc httpClientCfg, gitlabURL string) (*http.Transport, string, error) {
	certPool, err := x509.SystemCertPool()
	if err != nil {
		certPool = x509.NewCertPool()
	}

	if hcc.caFile != "" {
		addCertToPool(certPool, hcc.caFile)
	}

	if hcc.caPath != "" {
		fis, _ := os.ReadDir(hcc.caPath)
		for _, fi := range fis {
			if fi.IsDir() {
				continue
			}

			addCertToPool(certPool, filepath.Join(hcc.caPath, fi.Name()))
		}
	}
	tlsConfig := &tls.Config{
		RootCAs:    certPool,
		MinVersion: tls.VersionTLS12,
	}

	if hcc.HaveCertAndKey() {
		cert, loadErr := tls.LoadX509KeyPair(hcc.certPath, hcc.keyPath)
		if loadErr != nil {
			return nil, "", loadErr
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	return transport, gitlabURL, err
}

func addCertToPool(certPool *x509.CertPool, fileName string) {
	cert, err := os.ReadFile(filepath.Clean(fileName))
	if err == nil {
		certPool.AppendCertsFromPEM(cert)
	}
}

func buildHTTPTransport(gitlabURL string) (*http.Transport, string) {
	return &http.Transport{}, gitlabURL
}

func readTimeout(timeoutSeconds uint64) time.Duration {
	if timeoutSeconds == 0 || timeoutSeconds > math.MaxInt64 {
		timeoutSeconds = defaultReadTimeoutSeconds
	}

	return time.Duration(timeoutSeconds) * time.Second // #nosec G115
}
