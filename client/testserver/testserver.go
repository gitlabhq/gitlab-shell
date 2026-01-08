package testserver

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/testhelper"
)

// TestRequestHandler defines a test HTTP request handler with a path and handler function.
type TestRequestHandler struct {
	Path    string
	Handler func(w http.ResponseWriter, r *http.Request)
}

// StartSocketHTTPServer starts a socket based HTTP server
func StartSocketHTTPServer(t *testing.T, handlers []TestRequestHandler) string {
	t.Helper()

	// We can't use t.TempDir() here because it will create a directory that
	// far exceeds the 108 character limit which results in the socket failing
	// to be created.
	//
	// See https://gitlab.com/gitlab-org/gitlab-shell/-/issues/696#note_1664726924
	// for more detail.
	tempDir, err := os.MkdirTemp("", "http")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(tempDir)) })

	testSocket := path.Join(tempDir, "internal.sock")
	err = os.MkdirAll(filepath.Dir(testSocket), 0700)
	require.NoError(t, err)

	socketListener, err := net.Listen("unix", testSocket)
	require.NoError(t, err)

	server := http.Server{
		Handler:           buildHandler(handlers),
		ReadHeaderTimeout: 10 * time.Second,
		// We'll put this server through some nasty stuff we don't want
		// in our test output
		ErrorLog: log.New(io.Discard, "", 0),
	}
	go func() {
		_ = server.Serve(socketListener)
	}()

	url := "http+unix://" + testSocket

	return url
}

// StartHTTPServer starts a TCP based HTTP server
func StartHTTPServer(t *testing.T, handlers []TestRequestHandler) string {
	t.Helper()

	server := httptest.NewServer(buildHandler(handlers))
	t.Cleanup(func() { server.Close() })

	return server.URL
}

// StartRetryHTTPServer starts a TCP based HTTP server with retry capabilities
func StartRetryHTTPServer(t *testing.T, handlers []TestRequestHandler) string {
	attempts := map[string]int{}

	retryMiddileware := func(next func(w http.ResponseWriter, r *http.Request)) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts[r.URL.String()+r.Method]++
			if attempts[r.URL.String()+r.Method] == 1 {
				w.WriteHeader(500)
				return
			}

			http.HandlerFunc(next).ServeHTTP(w, r)
		})
	}
	t.Helper()

	h := http.NewServeMux()

	for _, handler := range handlers {
		h.Handle(handler.Path, retryMiddileware(handler.Handler))
	}

	server := httptest.NewServer(h)
	t.Cleanup(func() { server.Close() })

	return server.URL
}

// StartHTTPSServer starts a TCP based HTTPS capable server
func StartHTTPSServer(t *testing.T, handlers []TestRequestHandler, clientCAPath string) string {
	t.Helper()

	testRoot := testhelper.PrepareTestRootDir(t)

	crt := path.Join(testRoot, "certs/valid/server.crt")
	key := path.Join(testRoot, "certs/valid/server.key")

	server := httptest.NewUnstartedServer(buildHandler(handlers))
	cer, err := tls.LoadX509KeyPair(crt, key)
	require.NoError(t, err)

	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{cer},
		MinVersion:   tls.VersionTLS12,
	}

	if clientCAPath != "" {
		caCert, err := os.ReadFile(filepath.Clean(clientCAPath))
		require.NoError(t, err)

		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)

		server.TLS.ClientCAs = caCertPool
		server.TLS.ClientAuth = tls.RequireAndVerifyClientCert
	}

	server.StartTLS()

	t.Cleanup(func() { server.Close() })

	return server.URL
}

func buildHandler(handlers []TestRequestHandler) http.Handler {
	h := http.NewServeMux()

	for _, handler := range handlers {
		h.HandleFunc(handler.Path, handler.Handler)
	}

	return h
}
