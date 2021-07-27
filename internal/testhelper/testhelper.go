package testhelper

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"runtime"
	"testing"
	"time"

	"github.com/otiai10/copy"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/require"
)

var (
	TestRoot, _ = ioutil.TempDir("", "test-gitlab-shell")
)

func TempEnv(env map[string]string) func() {
	var original = make(map[string]string)
	for key, value := range env {
		original[key] = os.Getenv(key)
		os.Setenv(key, value)
	}

	return func() {
		for key, originalValue := range original {
			os.Setenv(key, originalValue)
		}
	}
}

func PrepareTestRootDir(t *testing.T) {
	t.Helper()

	require.NoError(t, os.MkdirAll(TestRoot, 0700))

	t.Cleanup(func() { os.RemoveAll(TestRoot) })

	require.NoError(t, copyTestData())

	oldWd, err := os.Getwd()
	require.NoError(t, err)

	t.Cleanup(func() { os.Chdir(oldWd) })

	require.NoError(t, os.Chdir(TestRoot))
}

func copyTestData() error {
	testDataDir, err := getTestDataDir()
	if err != nil {
		return err
	}

	testdata := path.Join(testDataDir, "testroot")

	return copy.Copy(testdata, TestRoot)
}

func getTestDataDir() (string, error) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("Could not get caller info")
	}

	return path.Join(path.Dir(currentFile), "testdata"), nil
}

func Setenv(key, value string) (func(), error) {
	oldValue := os.Getenv(key)
	err := os.Setenv(key, value)
	return func() { os.Setenv(key, oldValue) }, err
}

func SetupLogger() *test.Hook {
	logger, hook := test.NewNullLogger()
	logrus.SetOutput(logger.Writer())

	return hook
}

// logrus fires a Goroutine to write the output log, but there's no way to
// flush all outstanding hooks to fire. We just wait up to a second
// for an event to appear.
func WaitForLogEvent(hook *test.Hook) bool {
	for i := 0; i < 10; i++ {
		if entry := hook.LastEntry(); entry != nil {
			return true
		}

		time.Sleep(100 * time.Millisecond)
	}

	return false
}
