package testserver

import (
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	tempDir, _ = os.MkdirTemp("", "gitlab-shell-test-api")
	testSocket = path.Join(tempDir, "internal.sock")
)

type TestRequestHandler struct {
	Path    string
	Handler func(w http.ResponseWriter, r *http.Request)
}

func StartSocketHttpServer(t *testing.T, handlers []TestRequestHandler) string {
	t.Helper()

	err := os.MkdirAll(filepath.Dir(testSocket), 0700)
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(tempDir) })

	socketListener, err := net.Listen("unix", testSocket)
	require.NoError(t, err)

	server := http.Server{
		Handler: buildHandler(handlers),
		// We'll put this server through some nasty stuff we don't want
		// in our test output
		ErrorLog: log.New(io.Discard, "", 0),
	}
	go server.Serve(socketListener)

	url := "http+unix://" + testSocket

	return url
}

func StartHttpServer(t *testing.T, handlers []TestRequestHandler) string {
	t.Helper()

	server := httptest.NewServer(buildHandler(handlers))
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
