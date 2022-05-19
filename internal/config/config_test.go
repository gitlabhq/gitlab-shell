package config

import (
	"os"
	"testing"
	"time"

	yaml "gopkg.in/yaml.v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/internal/testhelper"
)

func TestConfigApplyGlobalState(t *testing.T) {
	t.Cleanup(testhelper.TempEnv(map[string]string{"SSL_CERT_DIR": "unmodified"}))

	config := &Config{SslCertDir: ""}
	config.ApplyGlobalState()

	require.Equal(t, "unmodified", os.Getenv("SSL_CERT_DIR"))

	config.SslCertDir = "foo"
	config.ApplyGlobalState()

	require.Equal(t, "foo", os.Getenv("SSL_CERT_DIR"))
}

func TestCustomPrometheusMetrics(t *testing.T) {
	url := testserver.StartHttpServer(t, []testserver.TestRequestHandler{})

	config := &Config{GitlabUrl: url}
	client, err := config.HttpClient()
	require.NoError(t, err)

	_, err = client.Get(url)
	require.NoError(t, err)

	ms, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)

	var actualNames []string
	for _, m := range ms[0:10] {
		actualNames = append(actualNames, m.GetName())
	}

	expectedMetricNames := []string{
		"gitlab_shell_http_in_flight_requests",
		"gitlab_shell_http_request_duration_seconds",
		"gitlab_shell_http_requests_total",
		"gitlab_shell_sshd_canceled_sessions",
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
	testhelper.PrepareTestRootDir(t)

	cfg, err := NewFromDir(testhelper.TestRoot)
	require.NoError(t, err)

	require.Equal(t, 10 * time.Second, time.Duration(cfg.Server.GracePeriod))
	require.Equal(t, 1 * time.Minute, time.Duration(cfg.Server.ClientAliveInterval))
}

func TestYAMLDuration(t *testing.T) {
	testCases := []struct{
		desc string
		data string
		duration time.Duration
	}{
		{"seconds assumed by default", "duration: 10", 10 * time.Second},
		{"milliseconds are parsed", "duration: 500ms", 500 * time.Millisecond},
		{"minutes are parsed", "duration: 1m", 1 * time.Minute},
	}

	type durationCfg struct {
		Duration yamlDuration `yaml:"duration"`
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
