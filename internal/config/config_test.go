package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v3"

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
