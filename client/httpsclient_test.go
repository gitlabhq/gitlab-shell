package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/testhelper"
)

//go:generate openssl req -newkey rsa:4096 -new -nodes -x509 -days 3650 -out ../internal/testhelper/testdata/testroot/certs/client/server.crt -keyout ../internal/testhelper/testdata/testroot/certs/client/key.pem -subj "/C=US/ST=California/L=San Francisco/O=GitLab/OU=GitLab-Shell/CN=localhost"
func TestSuccessfulRequests(t *testing.T) {
	testRoot := testhelper.PrepareTestRootDir(t)

	testCases := []struct {
		desc                                        string
		caFile, caPath                              string
		clientCAPath, clientCertPath, clientKeyPath string // used for TLS client certs
	}{
		{
			desc:   "Valid CaFile",
			caFile: path.Join(testRoot, "certs/valid/server.crt"),
		},
		{
			desc:   "Valid CaPath",
			caPath: path.Join(testRoot, "certs/valid"),
			caFile: path.Join(testRoot, "certs/valid/server.crt"),
		},
		{
			desc:   "Invalid cert with self signed cert option enabled",
			caFile: path.Join(testRoot, "certs/valid/server.crt"),
		},
		{
			desc:   "Client certs with CA",
			caFile: path.Join(testRoot, "certs/valid/server.crt"),
			// Run the command "go generate httpsclient_test.go" to
			// regenerate the following test fixtures:
			clientCAPath:   path.Join(testRoot, "certs/client/server.crt"),
			clientCertPath: path.Join(testRoot, "certs/client/server.crt"),
			clientKeyPath:  path.Join(testRoot, "certs/client/key.pem"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			client, err := setupWithRequests(t, tc.caFile, tc.caPath, tc.clientCAPath, tc.clientCertPath, tc.clientKeyPath)
			require.NoError(t, err)

			response, err := client.Get(context.Background(), "/hello")
			require.NoError(t, err)
			require.NotNil(t, response)

			defer response.Body.Close()

			responseBody, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.Equal(t, "Hello", string(responseBody))
		})
	}
}

func TestFailedRequests(t *testing.T) {
	testRoot := testhelper.PrepareTestRootDir(t)

	testCases := []struct {
		desc                   string
		caFile                 string
		caPath                 string
		expectedCaFileNotFound bool
		expectedError          string
	}{
		{
			desc:          "Invalid CaFile",
			caFile:        path.Join(testRoot, "certs/invalid/server.crt"),
			expectedError: "Internal API unreachable",
		},
		{
			desc:                   "Missing CaFile",
			caFile:                 path.Join(testRoot, "certs/invalid/missing.crt"),
			expectedCaFileNotFound: true,
		},
		{
			desc:          "Invalid CaPath",
			caPath:        path.Join(testRoot, "certs/invalid"),
			expectedError: "Internal API unreachable",
		},
		{
			desc:          "Empty config",
			expectedError: "Internal API unreachable",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			client, err := setupWithRequests(t, tc.caFile, tc.caPath, "", "", "")
			if tc.expectedCaFileNotFound {
				require.Error(t, err)
				require.ErrorIs(t, err, ErrCafileNotFound)
			} else {
				resp, err := client.Get(context.Background(), "/hello")
				require.Error(t, err)

				defer resp.Body.Close()

				require.Equal(t, tc.expectedError, err.Error())
			}
		})
	}
}

func setupWithRequests(t *testing.T, caFile, caPath, clientCAPath, clientCertPath, clientKeyPath string) (*GitlabNetClient, error) {
	requests := []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/hello",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)

				fmt.Fprint(w, "Hello")
			},
		},
	}

	url := testserver.StartHTTPSServer(t, requests, clientCAPath)

	opts := defaultHTTPOpts
	if clientCertPath != "" && clientKeyPath != "" {
		opts = append(opts, WithClientCert(clientCertPath, clientKeyPath))
	}

	httpClient, err := NewHTTPClientWithOpts(url, "", caFile, caPath, 1, opts)
	if err != nil {
		return nil, err
	}

	client, err := NewGitlabNetClient("", "", "", httpClient)

	return client, err
}
