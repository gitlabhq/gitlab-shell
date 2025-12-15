package config

import (
	"os"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v3"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/testhelper"
)

func TestDefaultConfig(t *testing.T) {
	config := &Config{}

	require.False(t, config.LFSConfig.PureSSHProtocol)
}

func TestConfigApplyGlobalState(t *testing.T) {
	testhelper.TempEnv(t, map[string]string{"SSL_CERT_DIR": "unmodified"})

	config := &Config{SslCertDir: ""}
	config.ApplyGlobalState()

	require.Equal(t, "unmodified", os.Getenv("SSL_CERT_DIR"))

	config.SslCertDir = "foo"
	config.ApplyGlobalState()

	require.Equal(t, "foo", os.Getenv("SSL_CERT_DIR"))
}

func TestCustomPrometheusMetrics(t *testing.T) {
	url := testserver.StartHTTPServer(t, []testserver.TestRequestHandler{})

	config := &Config{GitlabUrl: url}
	client, err := config.HTTPClient()
	require.NoError(t, err)

	if client.RetryableHTTP != nil {
		_, err = client.RetryableHTTP.Get(url)
		require.NoError(t, err)
	}

	ms, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)

	var actualNames []string
	for _, m := range ms[0:9] {
		actualNames = append(actualNames, m.GetName())
	}

	expectedMetricNames := []string{
		"gitlab_shell_http_in_flight_requests",
		"gitlab_shell_http_request_duration_seconds",
		"gitlab_shell_http_requests_total",
		"gitlab_shell_sshd_concurrent_limited_sessions_total",
		"gitlab_shell_sshd_in_flight_connections",
		"gitlab_shell_sshd_session_duration_seconds",
		"gitlab_shell_sshd_session_established_duration_seconds",
		"gitlab_sli:shell_sshd_sessions:errors_total",
		"gitlab_sli:shell_sshd_sessions:total",
	}

	require.Equal(t, expectedMetricNames, actualNames)
}

func TestNewFromDir(t *testing.T) {
	testRoot := testhelper.PrepareTestRootDir(t)

	cfg, err := NewFromDir(testRoot)
	require.NoError(t, err)

	require.Equal(t, 10*time.Second, time.Duration(cfg.Server.GracePeriod))
	require.Equal(t, 1*time.Minute, time.Duration(cfg.Server.ClientAliveInterval))
	require.Equal(t, 500*time.Millisecond, time.Duration(cfg.Server.ProxyHeaderTimeout))
}

func TestYAMLDuration(t *testing.T) {
	testCases := []struct {
		desc     string
		data     string
		duration time.Duration
	}{
		{"seconds assumed by default", "duration: 10", 10 * time.Second},
		{"milliseconds are parsed", "duration: 500ms", 500 * time.Millisecond},
		{"minutes are parsed", "duration: 1m", 1 * time.Minute},
	}

	type durationCfg struct {
		Duration YamlDuration `yaml:"duration"`
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			var cfg durationCfg
			err := yaml.Unmarshal([]byte(tc.data), &cfg)
			require.NoError(t, err)

			require.Equal(t, tc.duration, time.Duration(cfg.Duration))
		})
	}
}

func TestClientRetryConfig(t *testing.T) {
	testCases := []struct {
		name                string
		yamlConfig          string
		expectedMaxAttempts int
		expectedMaxBackoff  float64
	}{
		{
			name: "default retry config",
			yamlConfig: `
gitlab_url: http://localhost
secret: test-secret
`,
			expectedMaxAttempts: 4,
			expectedMaxBackoff:  1.4,
		},
		{
			name: "custom retry config",
			yamlConfig: `
gitlab_url: http://localhost
secret: test-secret
retry_policy:
  max_attempts: 5
  max_backoff: 2.5
`,
			expectedMaxAttempts: 5,
			expectedMaxBackoff:  2.5,
		},
		{
			name: "partial retry config - only max_attempts",
			yamlConfig: `
gitlab_url: http://localhost
secret: test-secret
retry_policy:
  max_attempts: 3
`,
			expectedMaxAttempts: 3,
			expectedMaxBackoff:  1.4,
		},
		{
			name: "partial retry config - only max_backoff",
			yamlConfig: `
gitlab_url: http://localhost
secret: test-secret
retry_policy:
  max_backoff: 3.0
`,
			expectedMaxAttempts: 4,
			expectedMaxBackoff:  3.0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testRoot := t.TempDir()
			configPath := testRoot + "/config.yml"
			err := os.WriteFile(configPath, []byte(tc.yamlConfig), 0o644)
			require.NoError(t, err)

			cfg, err := NewFromDir(testRoot)
			require.NoError(t, err)

			require.Equal(t, tc.expectedMaxAttempts, cfg.RetryPolicy.GetMaxAttempts())
			require.InDelta(t, tc.expectedMaxBackoff, cfg.RetryPolicy.GetMaxBackoff(), 0.01)

			// Verify the retry config is passed to the Gitaly client
			require.Equal(t, tc.expectedMaxAttempts, cfg.GitalyClient.MaxAttempts)
			require.InDelta(t, tc.expectedMaxBackoff, cfg.GitalyClient.MaxBackoff, 0.01)
		})
	}
}
