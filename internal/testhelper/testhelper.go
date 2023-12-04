package testhelper

import (
	"fmt"
	"os"
	"path"
	"runtime"
	"testing"

	"github.com/otiai10/copy"
	"github.com/stretchr/testify/require"
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

func PrepareTestRootDir(t *testing.T) string {
	t.Helper()

	testRoot, err := os.MkdirTemp("", "root")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(testRoot, 0700))

	t.Cleanup(func() { require.NoError(t, os.RemoveAll(testRoot)) })

	require.NoError(t, copyTestData(testRoot))

	oldWd, err := os.Getwd()
	require.NoError(t, err)

	t.Cleanup(func() { os.Chdir(oldWd) })

	require.NoError(t, os.Chdir(testRoot))

	return testRoot
}

func copyTestData(testRoot string) error {
	testDataDir, err := getTestDataDir()
	if err != nil {
		return err
	}

	testdata := path.Join(testDataDir, "testroot")

	return copy.Copy(testdata, testRoot)
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
