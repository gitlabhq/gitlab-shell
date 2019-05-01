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

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/testhelper"
)

var (
	tempDir, _ = ioutil.TempDir("", "gitlab-shell-test-api")
	testSocket = path.Join(tempDir, "internal.sock")
)

type TestRequestHandler struct {
	Path    string
	Handler func(w http.ResponseWriter, r *http.Request)
}

func StartSocketHttpServer(handlers []TestRequestHandler) (func(), string, error) {
	if err := os.MkdirAll(filepath.Dir(testSocket), 0700); err != nil {
		return nil, "", err
	}

	socketListener, err := net.Listen("unix", testSocket)
	if err != nil {
		return nil, "", err
	}

	server := http.Server{
		Handler: buildHandler(handlers),
		// We'll put this server through some nasty stuff we don't want
		// in our test output
		ErrorLog: log.New(ioutil.Discard, "", 0),
	}
	go server.Serve(socketListener)

	url := "http+unix://" + testSocket

	return cleanupSocket, url, nil
}

func StartHttpServer(handlers []TestRequestHandler) (func(), string, error) {
	server := httptest.NewServer(buildHandler(handlers))

	return server.Close, server.URL, nil
}

func StartHttpsServer(handlers []TestRequestHandler) (func(), string, error) {
	crt := path.Join(testhelper.TestRoot, "certs/valid/server.crt")
	key := path.Join(testhelper.TestRoot, "certs/valid/server.key")

	server := httptest.NewUnstartedServer(buildHandler(handlers))
	cer, err := tls.LoadX509KeyPair(crt, key)

	if err != nil {
		return nil, "", err
	}

	server.TLS = &tls.Config{Certificates: []tls.Certificate{cer}}
	server.StartTLS()

	return server.Close, server.URL, nil
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
