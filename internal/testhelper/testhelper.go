package testhelper

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"runtime"
	"time"

	"github.com/otiai10/copy"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
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

func PrepareTestRootDir() (func(), error) {
	if err := os.MkdirAll(TestRoot, 0700); err != nil {
		return nil, err
	}

	var oldWd string
	cleanup := func() {
		if oldWd != "" {
			err := os.Chdir(oldWd)
			if err != nil {
				panic(err)
			}
		}

		if err := os.RemoveAll(TestRoot); err != nil {
			panic(err)
		}
	}

	if err := copyTestData(); err != nil {
		cleanup()
		return nil, err
	}

	oldWd, err := os.Getwd()
	if err != nil {
		cleanup()
		return nil, err
	}

	if err := os.Chdir(TestRoot); err != nil {
		cleanup()
		return nil, err
	}

	return cleanup, nil
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

		time.Sleep(100*time.Millisecond)
	}

	return false
}
