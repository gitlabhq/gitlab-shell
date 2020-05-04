package client

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitlab-shell/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/internal/testhelper"
)

func TestSuccessfulRequests(t *testing.T) {
	testCases := []struct {
		desc           string
		caFile, caPath string
		selfSigned     bool
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
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			client, cleanup := setupWithRequests(t, tc.caFile, tc.caPath, tc.selfSigned)
			defer cleanup()

			response, err := client.Get("/hello")
			require.NoError(t, err)
			require.NotNil(t, response)

			defer response.Body.Close()

			responseBody, err := ioutil.ReadAll(response.Body)
			assert.NoError(t, err)
			assert.Equal(t, string(responseBody), "Hello")
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
			client, cleanup := setupWithRequests(t, tc.caFile, tc.caPath, false)
			defer cleanup()

			_, err := client.Get("/hello")
			require.Error(t, err)

			assert.Equal(t, err.Error(), "Internal API unreachable")
		})
	}
}

func setupWithRequests(t *testing.T, caFile, caPath string, selfSigned bool) (*GitlabNetClient, func()) {
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

	url, cleanup := testserver.StartHttpsServer(t, requests)

	httpClient := NewHTTPClient(url, caFile, caPath, selfSigned, 1)

	client, err := NewGitlabNetClient("", "", "", httpClient)
	require.NoError(t, err)

	return client, cleanup
}
