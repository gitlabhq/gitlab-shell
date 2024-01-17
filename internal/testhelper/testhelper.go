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

func TempEnv(t *testing.T, env map[string]string) {
	for key, value := range env {
		t.Setenv(key, value)
	}
}

func PrepareTestRootDir(t *testing.T) string {
	t.Helper()

	testRoot := t.TempDir()
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
