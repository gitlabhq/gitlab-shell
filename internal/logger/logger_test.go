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
	tmpFile, err := os.CreateTemp(os.TempDir(), "logtest-")
	require.NoError(t, err)
	defer tmpFile.Close()

	config := config.Config{
		LogFile:   tmpFile.Name(),
		LogFormat: "json",
	}

	closer := Configure(&config)
	defer closer.Close()

	log.Info("this is a test")
	log.WithFields(log.Fields{}).Debug("debug log message")

	tmpFile.Close()

	data, err := os.ReadFile(tmpFile.Name())
	dataStr := string(data)
	require.NoError(t, err)
	require.Contains(t, dataStr, `"msg":"this is a test"`)
	require.NotContains(t, dataStr, `"msg":"debug log message"`)
	require.NotContains(t, dataStr, `"msg":"unknown log level`)
}

func TestConfigureWithDebugLogLevel(t *testing.T) {
	tmpFile, err := os.CreateTemp(os.TempDir(), "logtest-")
	require.NoError(t, err)
	defer tmpFile.Close()

	config := config.Config{
		LogFile:   tmpFile.Name(),
		LogFormat: "json",
		LogLevel:  "debug",
	}

	closer := Configure(&config)
	defer closer.Close()

	log.WithFields(log.Fields{}).Debug("debug log message")

	tmpFile.Close()

	data, err := os.ReadFile(tmpFile.Name())
	require.NoError(t, err)
	require.Contains(t, string(data), `msg":"debug log message"`)
}

func TestConfigureWithPermissionError(t *testing.T) {
	tmpPath, err := os.MkdirTemp(os.TempDir(), "logtest-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpPath)

	config := config.Config{
		LogFile:   tmpPath,
		LogFormat: "json",
	}

	closer := Configure(&config)
	defer closer.Close()

	log.Info("this is a test")
}

func TestLogInUTC(t *testing.T) {
	tmpFile, err := os.CreateTemp(os.TempDir(), "logtest-")
	require.NoError(t, err)
	defer tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	config := config.Config{
		LogFile:   tmpFile.Name(),
		LogFormat: "json",
	}

	closer := Configure(&config)
	defer closer.Close()

	log.Info("this is a test")

	data, err := os.ReadFile(tmpFile.Name())
	require.NoError(t, err)

	utc := `[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z`
	r, e := regexp.MatchString(utc, string(data))

	require.NoError(t, e)
	require.True(t, r)
}
