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

	config := &Config{GitlabURL: url}
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

func TestTopologyServiceConfig(t *testing.T) {
	t.Run("default test config has topology_service disabled", func(t *testing.T) {
		testRoot := testhelper.PrepareTestRootDir(t)
		cfg, err := NewFromDir(testRoot)
		require.NoError(t, err)
		require.False(t, cfg.TopologyService.Enabled)
	})

	t.Run("parses full topology_service configuration from YAML", func(t *testing.T) {
		yamlData := `
topology_service:
  enabled: true
  address: "topology.example.com:443"
  classify_type: "first_cell"
  timeout: 10s
  tls:
    enabled: true
    ca_file: "/path/to/ca.crt"
    cert_file: "/path/to/cert.crt"
    key_file: "/path/to/key.pem"
    server_name: "topology.example.com"
    insecure_skip_verify: true
`
		var cfg Config
		require.NoError(t, yaml.Unmarshal([]byte(yamlData), &cfg))

		ts := cfg.TopologyService
		require.True(t, ts.Enabled)
		require.Equal(t, "topology.example.com:443", ts.Address)
		require.Equal(t, "first_cell", ts.ClassifyType)
		require.Equal(t, 10*time.Second, ts.Timeout)
		require.True(t, ts.TLS.Enabled)
		require.Equal(t, "/path/to/ca.crt", ts.TLS.CAFile)
		require.Equal(t, "/path/to/cert.crt", ts.TLS.CertFile)
		require.Equal(t, "/path/to/key.pem", ts.TLS.KeyFile)
		require.Equal(t, "topology.example.com", ts.TLS.ServerName)
		require.True(t, ts.TLS.InsecureSkipVerify)
	})
}

func TestTopologyServiceConfigValidation(t *testing.T) {
	t.Run("newFromFile rejects invalid topology config", func(t *testing.T) {
		// Create a temporary directory with an invalid config
		tmpDir := t.TempDir()
		configPath := tmpDir + "/config.yml"
		secretPath := tmpDir + "/.gitlab_shell_secret"

		// Write secret file
		require.NoError(t, os.WriteFile(secretPath, []byte("test-secret"), 0o600))

		// Write config with enabled topology but missing address
		invalidConfig := `
topology_service:
  enabled: true
`
		require.NoError(t, os.WriteFile(configPath, []byte(invalidConfig), 0o600))

		_, err := NewFromDir(tmpDir)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid topology_service config")
		require.Contains(t, err.Error(), "address is required")
	})
}
