package gitlabnet

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/internal/testhelper"
)

func TestSuccessfulRequests(t *testing.T) {
	testCases := []struct {
		desc   string
		config *config.Config
	}{
		{
			desc: "Valid CaFile",
			config: &config.Config{
				HttpSettings: config.HttpSettingsConfig{CaFile: path.Join(testhelper.TestRoot, "certs/valid/server.crt")},
			},
		},
		{
			desc: "Valid CaPath",
			config: &config.Config{
				HttpSettings: config.HttpSettingsConfig{CaPath: path.Join(testhelper.TestRoot, "certs/valid")},
			},
		},
		{
			desc: "Self signed cert option enabled",
			config: &config.Config{
				HttpSettings: config.HttpSettingsConfig{SelfSignedCert: true},
			},
		},
		{
			desc: "Invalid cert with self signed cert option enabled",
			config: &config.Config{
				HttpSettings: config.HttpSettingsConfig{SelfSignedCert: true, CaFile: path.Join(testhelper.TestRoot, "certs/valid/server.crt")},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			client, cleanup := setupWithRequests(t, tc.config)
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
		config *config.Config
	}{
		{
			desc: "Invalid CaFile",
			config: &config.Config{
				HttpSettings: config.HttpSettingsConfig{CaFile: path.Join(testhelper.TestRoot, "certs/invalid/server.crt")},
			},
		},
		{
			desc: "Invalid CaPath",
			config: &config.Config{
				HttpSettings: config.HttpSettingsConfig{CaPath: path.Join(testhelper.TestRoot, "certs/invalid")},
			},
		},
		{
			desc:   "Empty config",
			config: &config.Config{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			client, cleanup := setupWithRequests(t, tc.config)
			defer cleanup()

			_, err := client.Get("/hello")
			require.Error(t, err)

			assert.Equal(t, err.Error(), "Internal API unreachable")
		})
	}
}

func setupWithRequests(t *testing.T, config *config.Config) (*GitlabClient, func()) {
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

	config.GitlabUrl = url
	client, err := GetClient(config)
	require.NoError(t, err)

	return client, cleanup
}
