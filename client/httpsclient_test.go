package client

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitlab-shell/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/internal/testhelper"
)

//go:generate openssl req -newkey rsa:4096 -new -nodes -x509 -days 3650 -out ../internal/testhelper/testdata/testroot/certs/client/server.crt -keyout ../internal/testhelper/testdata/testroot/certs/client/key.pem -subj "/C=US/ST=California/L=San Francisco/O=GitLab/OU=GitLab-Shell/CN=localhost"
func TestSuccessfulRequests(t *testing.T) {
	testCases := []struct {
		desc                                        string
		caFile, caPath                              string
		selfSigned                                  bool
		clientCAPath, clientCertPath, clientKeyPath string // used for TLS client certs
	}{
		{
			desc:   "Valid CaFile",
			caFile: path.Join(testhelper.TestRoot, "certs/valid/server.crt"),
		},
		{
			desc:   "Valid CaPath",
			caPath: path.Join(testhelper.TestRoot, "certs/valid"),
		},
		{
			desc:       "Self signed cert option enabled",
			selfSigned: true,
		},
		{
			desc:       "Invalid cert with self signed cert option enabled",
			caFile:     path.Join(testhelper.TestRoot, "certs/valid/server.crt"),
			selfSigned: true,
		},
		{
			desc:   "Client certs with CA",
			caFile: path.Join(testhelper.TestRoot, "certs/valid/server.crt"),
			// Run the command "go generate httpsclient_test.go" to
			// regenerate the following test fixtures:
			clientCAPath:   path.Join(testhelper.TestRoot, "certs/client/server.crt"),
			clientCertPath: path.Join(testhelper.TestRoot, "certs/client/server.crt"),
			clientKeyPath:  path.Join(testhelper.TestRoot, "certs/client/key.pem"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			client, cleanup := setupWithRequests(t, tc.caFile, tc.caPath, tc.clientCAPath, tc.clientCertPath, tc.clientKeyPath, tc.selfSigned)
			defer cleanup()

			response, err := client.Get(context.Background(), "/hello")
			require.NoError(t, err)
			require.NotNil(t, response)

			defer response.Body.Close()

			responseBody, err := ioutil.ReadAll(response.Body)
			require.NoError(t, err)
			require.Equal(t, string(responseBody), "Hello")
		})
	}
}

func TestFailedRequests(t *testing.T) {
	testCases := []struct {
		desc   string
		caFile string
		caPath string
	}{
		{
			desc:   "Invalid CaFile",
			caFile: path.Join(testhelper.TestRoot, "certs/invalid/server.crt"),
		},
		{
			desc:   "Invalid CaPath",
			caPath: path.Join(testhelper.TestRoot, "certs/invalid"),
		},
		{
			desc: "Empty config",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			client, cleanup := setupWithRequests(t, tc.caFile, tc.caPath, "", "", "", false)
			defer cleanup()

			_, err := client.Get(context.Background(), "/hello")
			require.Error(t, err)

			require.Equal(t, err.Error(), "Internal API unreachable")
		})
	}
}

func setupWithRequests(t *testing.T, caFile, caPath, clientCAPath, clientCertPath, clientKeyPath string, selfSigned bool) (*GitlabNetClient, func()) {
	testDirCleanup, err := testhelper.PrepareTestRootDir()
	require.NoError(t, err)
	defer testDirCleanup()

	requests := []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/hello",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodGet, r.Method)

				fmt.Fprint(w, "Hello")
			},
		},
	}

	url, cleanup := testserver.StartHttpsServer(t, requests, clientCAPath)

	var opts []HTTPClientOpt
	if clientCertPath != "" && clientKeyPath != "" {
		opts = append(opts, WithClientCert(clientCertPath, clientKeyPath))
	}

	httpClient, err := NewHTTPClientWithOpts(url, "", caFile, caPath, selfSigned, 1, opts)
	require.NoError(t, err)

	client, err := NewGitlabNetClient("", "", "", httpClient)
	require.NoError(t, err)

	return client, cleanup
}
