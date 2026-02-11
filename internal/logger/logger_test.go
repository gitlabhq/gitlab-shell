package logger

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
)

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

	closer := ConfigureLogger(&config)
	if closer != nil {
		defer MustClose(t, closer)
	}
	slog.Info("this is a test")
	slog.Debug("debug log message")

	data, err := os.ReadFile(tmpFile)
	dataStr := string(data)
	require.NoError(t, err)
	require.Contains(t, dataStr, `"msg":"this is a test"`)
	require.Contains(t, dataStr, `"msg":"debug log message"`)
}

// MustClose calls Close() on the Closer and fails the test in case it returns
// an error. This function is useful when closing via `defer`, as a simple
// `defer require.NoError(t, closer.Close())` would cause `closer.Close()` to
// be executed early already.
func MustClose(tb testing.TB, closer io.Closer) {
	require.NoError(tb, closer.Close())
}

func TestConfigureLoggerDirectoryFailure(t *testing.T) {
	tempDir := t.TempDir()

	config := config.Config{
		LogFile:   tempDir,
		LogFormat: "json",
	}

	fileInfo, err := os.Stat(tempDir)
	require.NoError(t, err)
	assert.True(t, fileInfo.IsDir())

	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	closer := ConfigureLogger(&config)
	assert.Nil(t, closer)
	slog.Info("this is a test")

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	os.Stderr = old

	assert.Contains(t, buf.String(), "failed to configure log file", "capture the error in stderr")
	assert.Contains(t, buf.String(), "this is a test", "we should still be logging to stderr in this case")
}
