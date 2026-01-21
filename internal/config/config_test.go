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

func TestApplyDefaults(t *testing.T) {
	testCases := []struct {
		name       string
		yamlConfig string
	}{
		{
			name:       "blank config preserves defaults",
			yamlConfig: ``,
		},
		{
			name: "minimal config preserves defaults",
			yamlConfig: `
gitlab_url: http://localhost
secret: test-secret
`,
		},
		{
			name: "partial sshd config preserves other defaults",
			yamlConfig: `
gitlab_url: http://localhost
secret: test-secret
sshd:
  listen: "[::]:2222"
`,
		},
		{
			name: "empty sshd block preserves defaults",
			yamlConfig: `
gitlab_url: http://localhost
secret: test-secret
sshd:
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testRoot := t.TempDir()
			configPath := testRoot + "/config.yml"
			err := os.WriteFile(configPath, []byte(tc.yamlConfig), 0o644)
			require.NoError(t, err)

			secretPath := testRoot + "/.gitlab_shell_secret"
			err = os.WriteFile(secretPath, []byte("test-secret"), 0o644)
			require.NoError(t, err)

			cfg, err := NewFromDir(testRoot)
			require.NoError(t, err)

			require.Equal(t, testRoot+"/"+DefaultConfig.LogFile, cfg.LogFile)
			require.Equal(t, DefaultConfig.LogFormat, cfg.LogFormat)
			require.Equal(t, DefaultConfig.LogLevel, cfg.LogLevel)
			require.Equal(t, DefaultConfig.User, cfg.User)

			require.Equal(t, DefaultServerConfig.WebListen, cfg.Server.WebListen)
			require.Equal(t, DefaultServerConfig.ConcurrentSessionsLimit, cfg.Server.ConcurrentSessionsLimit)
			require.Equal(t, DefaultServerConfig.GracePeriod, cfg.Server.GracePeriod)
			require.Equal(t, DefaultServerConfig.ClientAliveInterval, cfg.Server.ClientAliveInterval)
			require.Equal(t, DefaultServerConfig.ProxyHeaderTimeout, cfg.Server.ProxyHeaderTimeout)
			require.Equal(t, DefaultServerConfig.LoginGraceTime, cfg.Server.LoginGraceTime)
			require.Equal(t, DefaultServerConfig.ReadinessProbe, cfg.Server.ReadinessProbe)
			require.Equal(t, DefaultServerConfig.LivenessProbe, cfg.Server.LivenessProbe)
			require.Equal(t, DefaultServerConfig.HostKeyFiles, cfg.Server.HostKeyFiles)
		})
	}
}

func TestApplyDefaultsWithCustomValues(t *testing.T) {
	yamlConfig := `
gitlab_url: http://localhost
secret: test-secret
log_file: custom.log
log_format: text
log_level: debug
user: custom-user
sshd:
  listen: "[::]:2222"
  web_listen: "localhost:9999"
  concurrent_sessions_limit: 20
  grace_period: 30s
  client_alive_interval: 30s
  proxy_header_timeout: 1s
  login_grace_time: 120s
  readiness_probe: /ready
  liveness_probe: /live
  host_key_files:
    - /custom/key
`
	testRoot := t.TempDir()
	configPath := testRoot + "/config.yml"
	err := os.WriteFile(configPath, []byte(yamlConfig), 0o644)
	require.NoError(t, err)

	cfg, err := NewFromDir(testRoot)
	require.NoError(t, err)

	require.Equal(t, testRoot+"/custom.log", cfg.LogFile)
	require.Equal(t, "text", cfg.LogFormat)
	require.Equal(t, "debug", cfg.LogLevel)
	require.Equal(t, "custom-user", cfg.User)

	require.Equal(t, "[::]:2222", cfg.Server.Listen)
	require.Equal(t, "localhost:9999", cfg.Server.WebListen)
	require.Equal(t, int64(20), cfg.Server.ConcurrentSessionsLimit)
	require.Equal(t, YamlDuration(30*time.Second), cfg.Server.GracePeriod)
	require.Equal(t, YamlDuration(30*time.Second), cfg.Server.ClientAliveInterval)
	require.Equal(t, YamlDuration(1*time.Second), cfg.Server.ProxyHeaderTimeout)
	require.Equal(t, YamlDuration(120*time.Second), cfg.Server.LoginGraceTime)
	require.Equal(t, "/ready", cfg.Server.ReadinessProbe)
	require.Equal(t, "/live", cfg.Server.LivenessProbe)
	require.Equal(t, []string{"/custom/key"}, cfg.Server.HostKeyFiles)
}

func TestGitalyClientRetryConfig(t *testing.T) {
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

			require.Equal(t, tc.expectedMaxAttempts, cfg.GitalyRetryPolicy.MaxAttempts)
			require.InDelta(t, tc.expectedMaxBackoff, cfg.GitalyRetryPolicy.MaxBackoff, 0.01)

			// Verify the retry config is passed to the Gitaly client
			require.Equal(t, tc.expectedMaxAttempts, cfg.GitalyClient.MaxAttempts)
			require.InDelta(t, tc.expectedMaxBackoff, cfg.GitalyClient.MaxBackoff, 0.01)
		})
	}
}
