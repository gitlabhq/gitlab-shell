package logger

import (
	"io/ioutil"
	"os"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
)

func TestConfigure(t *testing.T) {
	tmpFile, err := ioutil.TempFile(os.TempDir(), "logtest-")
	require.NoError(t, err)
	defer tmpFile.Close()

	config := config.Config{
		LogFile:   tmpFile.Name(),
		LogFormat: "json",
	}

	Configure(&config)
	log.Info("this is a test")

	tmpFile.Close()

	data, err := ioutil.ReadFile(tmpFile.Name())
	require.NoError(t, err)
	require.True(t, strings.Contains(string(data), `msg":"this is a test"`))
}

func TestConfigureWithPermissionError(t *testing.T) {
	tmpPath, err := ioutil.TempDir(os.TempDir(), "logtest-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpPath)

	config := config.Config{
		LogFile:   tmpPath,
		LogFormat: "json",
	}

	Configure(&config)
	log.Info("this is a test")
}
