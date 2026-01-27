package logger

import (
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/labkit/log"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
)

func TestConfigure(t *testing.T) {
	tmpFile := createTempFile(t)

	config := config.Config{
		LogFile:   tmpFile,
		LogFormat: "json",
	}

	closer := Configure(&config)
	defer closer.Close()

	log.Info("this is a test")
	log.WithFields(log.Fields{}).Debug("debug log message")

	data, err := os.ReadFile(tmpFile)
	dataStr := string(data)
	require.NoError(t, err)
	require.Contains(t, dataStr, `"msg":"this is a test"`)
	require.NotContains(t, dataStr, `"msg":"debug log message"`)
	require.NotContains(t, dataStr, `"msg":"unknown log level`)
}

func TestConfigureWithDebugLogLevel(t *testing.T) {
	tmpFile := createTempFile(t)

	config := config.Config{
		LogFile:   tmpFile,
		LogFormat: "json",
		LogLevel:  "debug",
	}

	closer := Configure(&config)
	defer closer.Close()

	log.WithFields(log.Fields{}).Debug("debug log message")

	data, err := os.ReadFile(tmpFile)
	require.NoError(t, err)
	require.Contains(t, string(data), `msg":"debug log message"`)
}

func TestConfigureWithPermissionError(t *testing.T) {
	tempDir := t.TempDir()

	config := config.Config{
		LogFile:   tempDir,
		LogFormat: "json",
	}

	closer := Configure(&config)
	defer closer.Close()

	log.Info("this is a test")
}

func TestLogInUTC(t *testing.T) {
	tmpFile := createTempFile(t)

	config := config.Config{
		LogFile:   tmpFile,
		LogFormat: "json",
	}

	closer := Configure(&config)
	defer closer.Close()

	log.Info("this is a test")

	data, err := os.ReadFile(tmpFile)
	require.NoError(t, err)

	utc := `[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z`
	r, e := regexp.MatchString(utc, string(data))

	require.NoError(t, e)
	require.True(t, r)
}

func createTempFile(t *testing.T) string {
	t.Helper()

	tmpFile, err := os.CreateTemp(t.TempDir(), "logtest-")
	require.NoError(t, err)
	tmpFile.Close()

	return tmpFile.Name()
}

func TestConfigureLabkitV2Log(t *testing.T) {
	tmpFile := createTempFile(t)
	config := config.Config{
		LogFile:   tmpFile,
		LogFormat: "json",
		LogLevel:  "debug",
	}

	logger := ConfigureLogger(&config)
	logger.Info("this is a test")
	logger.Debug("debug log message")

	data, err := os.ReadFile(tmpFile)
	dataStr := string(data)
	require.NoError(t, err)
	require.Contains(t, dataStr, `"msg":"this is a test"`)
	require.Contains(t, dataStr, `"msg":"debug log message"`)
}
