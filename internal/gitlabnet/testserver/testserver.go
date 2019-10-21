package testserver

import (
	"crypto/tls"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/internal/testhelper"
)

var (
	tempDir, _ = ioutil.TempDir("", "gitlab-shell-test-api")
	testSocket = path.Join(tempDir, "internal.sock")
)

type TestRequestHandler struct {
	Path    string
	Handler func(w http.ResponseWriter, r *http.Request)
}

func StartSocketHttpServer(t *testing.T, handlers []TestRequestHandler) (string, func()) {
	err := os.MkdirAll(filepath.Dir(testSocket), 0700)
	require.NoError(t, err)

	socketListener, err := net.Listen("unix", testSocket)
	require.NoError(t, err)

	server := http.Server{
		Handler: buildHandler(handlers),
		// We'll put this server through some nasty stuff we don't want
		// in our test output
		ErrorLog: log.New(ioutil.Discard, "", 0),
	}
	go server.Serve(socketListener)

	url := "http+unix://" + testSocket

	return url, cleanupSocket
}

func StartHttpServer(t *testing.T, handlers []TestRequestHandler) (string, func()) {
	server := httptest.NewServer(buildHandler(handlers))

	return server.URL, server.Close
}

func StartHttpsServer(t *testing.T, handlers []TestRequestHandler) (string, func()) {
	crt := path.Join(testhelper.TestRoot, "certs/valid/server.crt")
	key := path.Join(testhelper.TestRoot, "certs/valid/server.key")

	server := httptest.NewUnstartedServer(buildHandler(handlers))
	cer, err := tls.LoadX509KeyPair(crt, key)
	require.NoError(t, err)

	server.TLS = &tls.Config{Certificates: []tls.Certificate{cer}}
	server.StartTLS()

	return server.URL, server.Close
}

func cleanupSocket() {
	os.RemoveAll(tempDir)
}

func buildHandler(handlers []TestRequestHandler) http.Handler {
	h := http.NewServeMux()

	for _, handler := range handlers {
		h.HandleFunc(handler.Path, handler.Handler)
	}

	return h
}
