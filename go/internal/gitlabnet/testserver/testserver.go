package testserver

import (
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
)

var (
	tempDir, _ = ioutil.TempDir("", "gitlab-shell-test-api")
	TestSocket = path.Join(tempDir, "internal.sock")
)

type TestRequestHandler struct {
	Path    string
	Handler func(w http.ResponseWriter, r *http.Request)
}

func StartSocketHttpServer(handlers []TestRequestHandler) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(TestSocket), 0700); err != nil {
		return nil, err
	}

	socketListener, err := net.Listen("unix", TestSocket)
	if err != nil {
		return nil, err
	}

	server := http.Server{
		Handler: buildHandler(handlers),
		// We'll put this server through some nasty stuff we don't want
		// in our test output
		ErrorLog: log.New(ioutil.Discard, "", 0),
	}
	go server.Serve(socketListener)

	return cleanupSocket, nil
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
