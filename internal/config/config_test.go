package config

import (
	"fmt"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitlab-shell/internal/testhelper"
)

const (
	customSecret = "custom/my-contents-is-secret"
)

var (
	testRoot = testhelper.TestRoot
)

func TestParseConfig(t *testing.T) {
	cleanup, err := testhelper.PrepareTestRootDir()
	require.NoError(t, err)
	defer cleanup()

	testCases := []struct {
		yaml         string
		path         string
		format       string
		gitlabUrl    string
		secret       string
		httpSettings HttpSettingsConfig
	}{
		{
			path:   path.Join(testRoot, "gitlab-shell.log"),
			format: "text",
			secret: "default-secret-content",
		},
		{
			yaml:   "log_file: my-log.log",
			path:   path.Join(testRoot, "my-log.log"),
			format: "text",
			secret: "default-secret-content",
		},
		{
			yaml:   "log_file: /qux/my-log.log",
			path:   "/qux/my-log.log",
			format: "text",
			secret: "default-secret-content",
		},
		{
			yaml:   "log_format: json",
			path:   path.Join(testRoot, "gitlab-shell.log"),
			format: "json",
			secret: "default-secret-content",
		},
		{
			yaml:      "gitlab_url: http+unix://%2Fpath%2Fto%2Fgitlab%2Fgitlab.socket",
			path:      path.Join(testRoot, "gitlab-shell.log"),
			format:    "text",
			gitlabUrl: "http+unix:///path/to/gitlab/gitlab.socket",
			secret:    "default-secret-content",
		},
		{
			yaml:   fmt.Sprintf("secret_file: %s", customSecret),
			path:   path.Join(testRoot, "gitlab-shell.log"),
			format: "text",
			secret: "custom-secret-content",
		},
		{
			yaml:   fmt.Sprintf("secret_file: %s", path.Join(testRoot, customSecret)),
			path:   path.Join(testRoot, "gitlab-shell.log"),
			format: "text",
			secret: "custom-secret-content",
		},
		{
			yaml:   "secret: an inline secret",
			path:   path.Join(testRoot, "gitlab-shell.log"),
			format: "text",
			secret: "an inline secret",
		},
		{
			yaml:         "http_settings:\n  user: user_basic_auth\n  password: password_basic_auth\n  read_timeout: 500",
			path:         path.Join(testRoot, "gitlab-shell.log"),
			format:       "text",
			secret:       "default-secret-content",
			httpSettings: HttpSettingsConfig{User: "user_basic_auth", Password: "password_basic_auth", ReadTimeoutSeconds: 500},
		},
		{
			yaml:         "http_settings:\n  ca_file: /etc/ssl/cert.pem\n  ca_path: /etc/pki/tls/certs\n  self_signed_cert: true",
			path:         path.Join(testRoot, "gitlab-shell.log"),
			format:       "text",
			secret:       "default-secret-content",
			httpSettings: HttpSettingsConfig{CaFile: "/etc/ssl/cert.pem", CaPath: "/etc/pki/tls/certs", SelfSignedCert: true},
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("yaml input: %q", tc.yaml), func(t *testing.T) {
			cfg := Config{RootDir: testRoot}

			err := parseConfig([]byte(tc.yaml), &cfg)
			require.NoError(t, err)

			assert.Equal(t, tc.path, cfg.LogFile)
			assert.Equal(t, tc.format, cfg.LogFormat)
			assert.Equal(t, tc.gitlabUrl, cfg.GitlabUrl)
			assert.Equal(t, tc.secret, cfg.Secret)
			assert.Equal(t, tc.httpSettings, cfg.HttpSettings)
		})
	}
}
