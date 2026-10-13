package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeConfig(t *testing.T, dir, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yml"), []byte(content), 0o600))
}

func runWithEnv(t *testing.T, root, home string) (int, string, string) {
	t.Helper()
	t.Setenv("GITLAB_SHELL_DIR", root)
	t.Setenv("HOME", home)
	if home == "" {
		require.NoError(t, os.Unsetenv("HOME"))
	}

	var stdout, stderr bytes.Buffer
	status := run(&stdout, &stderr)
	return status, stdout.String(), stderr.String()
}

func requireKeyDir(t *testing.T, keyDir string) {
	t.Helper()
	info, err := os.Stat(keyDir)
	require.NoError(t, err)
	require.True(t, info.IsDir())
	require.Equal(t, os.FileMode(0o700), info.Mode().Perm())
}

func successOutput(keyDir string) string {
	return fmt.Sprintf("mkdir -p %s: OK\nchmod 700 %s: OK\n", keyDir, keyDir)
}

func requireSuccess(t *testing.T, root, home, printedDir, createdDir string) {
	t.Helper()
	status, stdout, stderr := runWithEnv(t, root, home)
	require.Equal(t, 0, status)
	require.Equal(t, successOutput(printedDir), stdout)
	require.Empty(t, stderr)
	requireKeyDir(t, createdDir)
}

func TestKeyDirectory(t *testing.T) {
	for authFile, want := range map[string]string{
		"":                  ".",
		"authorized_keys":   ".",
		"/authorized_keys":  "/",
		"a//b":              "a",
		"/":                 "/",
		"//authorized_keys": "/",
	} {
		t.Run(authFile, func(t *testing.T) {
			require.Equal(t, want, keyDirectory(authFile))
		})
	}
}

func TestRunCreatesAndSecuresDirectory(t *testing.T) {
	t.Run("auth file with trailing slash", func(t *testing.T) {
		root := t.TempDir()
		keyDir := filepath.Join(root, "a")
		writeConfig(t, root, fmt.Sprintf("auth_file: %s/\n", filepath.Join(keyDir, "keys")))

		requireSuccess(t, root, t.TempDir(), keyDir, keyDir)
		require.NoDirExists(t, filepath.Join(keyDir, "keys"))
	})

	t.Run("nested absolute auth file without a secret", func(t *testing.T) {
		root := t.TempDir()
		keyDir := filepath.Join(root, "a", "b", ".ssh")
		writeConfig(t, root, fmt.Sprintf("auth_file: %s/authorized_keys\n", keyDir))

		requireSuccess(t, root, t.TempDir(), keyDir, keyDir)
	})

	t.Run("symlink before parent traversal", func(t *testing.T) {
		root := t.TempDir()
		targetChild := filepath.Join(root, "target", "child")
		require.NoError(t, os.MkdirAll(targetChild, 0o755))
		require.NoError(t, os.Symlink(targetChild, filepath.Join(root, "alias")))
		keyDir := filepath.Join(root, "alias") + "/../keys"
		writeConfig(t, root, fmt.Sprintf("auth_file: %s/authorized_keys\n", keyDir))

		requireSuccess(t, root, t.TempDir(), keyDir, filepath.Join(root, "target", "keys"))
		require.NoDirExists(t, filepath.Join(root, "keys"))
	})

	t.Run("missing directory before parent traversal", func(t *testing.T) {
		root := t.TempDir()
		keyDir := filepath.Join(root, "a", "missing") + "/../keys"
		writeConfig(t, root, fmt.Sprintf("auth_file: %s/authorized_keys\n", keyDir))

		requireSuccess(t, root, t.TempDir(), keyDir, filepath.Join(root, "a", "keys"))
		require.DirExists(t, filepath.Join(root, "a", "missing"))
	})

	t.Run("existing directory", func(t *testing.T) {
		root := t.TempDir()
		keyDir := filepath.Join(root, ".ssh")
		require.NoError(t, os.Mkdir(keyDir, 0o755))
		writeConfig(t, root, fmt.Sprintf("auth_file: %s/authorized_keys\n", keyDir))

		requireSuccess(t, root, t.TempDir(), keyDir, keyDir)
	})
}

func TestRunDefaultsToHome(t *testing.T) {
	for _, tc := range []struct {
		name, config string
	}{
		{"missing key", "user: git\n"},
		{"empty quoted value", "auth_file: \"\"\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			home := t.TempDir()
			writeConfig(t, root, tc.config)
			keyDir := filepath.Join(home, ".ssh")

			requireSuccess(t, root, home, keyDir, keyDir)
		})
	}
}

func TestRunDefaultsToPasswdHomeWhenHomeUnset(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeConfig(t, root, "user: git\n")
	previous := userCurrent
	userCurrent = func() (*user.User, error) { return &user.User{HomeDir: home}, nil }
	t.Cleanup(func() { userCurrent = previous })
	keyDir := filepath.Join(home, ".ssh")

	requireSuccess(t, root, "", keyDir, keyDir)
}

func TestRunParsesAuthFile(t *testing.T) {
	t.Run("relative auth file", func(t *testing.T) {
		root := t.TempDir()
		cwd := t.TempDir()
		writeConfig(t, root, "auth_file: keys/authorized_keys\n")
		t.Chdir(cwd)
		requireSuccess(t, root, t.TempDir(), "keys", filepath.Join(cwd, "keys"))
		require.NoDirExists(t, filepath.Join(root, "keys"))
	})

	t.Run("literal tilde in relative auth file", func(t *testing.T) {
		root := t.TempDir()
		home := t.TempDir()
		writeConfig(t, root, "auth_file: ~/authorized_keys\n")
		t.Chdir(root)

		requireSuccess(t, root, home, "~", filepath.Join(root, "~"))
		require.NoDirExists(t, filepath.Join(home, ".ssh"))
	})
}

func TestRunSelectsConfigRoot(t *testing.T) {
	setup := func(t *testing.T) (string, string, string) {
		t.Helper()
		cwd := t.TempDir()
		explicit := t.TempDir()
		t.Chdir(cwd)
		cwdKeyDir := filepath.Join(cwd, "cwd-keys")
		explicitKeyDir := filepath.Join(explicit, "explicit-keys")
		writeConfig(t, cwd, fmt.Sprintf("auth_file: %s/authorized_keys\n", cwdKeyDir))
		writeConfig(t, explicit, fmt.Sprintf("auth_file: %s/authorized_keys\n", explicitKeyDir))
		return explicit, cwdKeyDir, explicitKeyDir
	}

	t.Run("GITLAB_SHELL_DIR set", func(t *testing.T) {
		explicit, cwdKeyDir, explicitKeyDir := setup(t)
		requireSuccess(t, explicit, t.TempDir(), explicitKeyDir, explicitKeyDir)
		require.NoDirExists(t, cwdKeyDir)
	})

	t.Run("GITLAB_SHELL_DIR empty uses cwd", func(t *testing.T) {
		_, cwdKeyDir, explicitKeyDir := setup(t)
		requireSuccess(t, "", t.TempDir(), cwdKeyDir, cwdKeyDir)
		require.NoDirExists(t, explicitKeyDir)
	})

	t.Run("symlink before parent traversal", func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(root, "target")
		require.NoError(t, os.MkdirAll(filepath.Join(target, "child"), 0o755))
		require.NoError(t, os.Symlink(filepath.Join(target, "child"), filepath.Join(root, "alias")))
		keyDir := filepath.Join(target, "keys")
		writeConfig(t, target, fmt.Sprintf("auth_file: %s/authorized_keys\n", keyDir))
		configRoot := filepath.Join(root, "alias") + "/.."

		requireSuccess(t, configRoot, t.TempDir(), keyDir, keyDir)
		require.NoDirExists(t, filepath.Join(root, "keys"))
	})
}

func TestRunReportsFailures(t *testing.T) {
	t.Run("missing config", func(t *testing.T) {
		status, stdout, stderr := runWithEnv(t, t.TempDir(), t.TempDir())
		require.Equal(t, 1, status)
		require.Empty(t, stdout)
		require.Contains(t, stderr, "config.yml")
		require.True(t, strings.HasSuffix(stderr, "support/make_necessary_dirs failed\n"))
	})

	t.Run("invalid YAML", func(t *testing.T) {
		root := t.TempDir()
		writeConfig(t, root, "auth_file: [unclosed\n")
		status, stdout, stderr := runWithEnv(t, root, t.TempDir())
		require.Equal(t, 1, status)
		require.Empty(t, stdout)
		require.Contains(t, stderr, "config.yml")
		require.True(t, strings.HasSuffix(stderr, "support/make_necessary_dirs failed\n"))
	})

	t.Run("mkdir fails on a file parent", func(t *testing.T) {
		root := t.TempDir()
		parent := filepath.Join(root, "f")
		require.NoError(t, os.WriteFile(parent, nil, 0o600))
		keyDir := filepath.Join(parent, ".ssh")
		writeConfig(t, root, fmt.Sprintf("auth_file: %s/authorized_keys\n", keyDir))
		status, stdout, stderr := runWithEnv(t, root, t.TempDir())
		require.Equal(t, 1, status)
		require.Equal(t, fmt.Sprintf("mkdir -p %s: Failed\n", keyDir), stdout)
		require.Contains(t, stderr, "not a directory")
		require.True(t, strings.HasSuffix(stderr, "support/make_necessary_dirs failed\n"))
	})

	t.Run("chmod fails", func(t *testing.T) {
		root := t.TempDir()
		keyDir := filepath.Join(root, ".ssh")
		writeConfig(t, root, fmt.Sprintf("auth_file: %s/authorized_keys\n", keyDir))
		previous := chmod
		chmod = func(string, os.FileMode) error { return errors.New("chmod rejected") }
		t.Cleanup(func() { chmod = previous })
		status, stdout, stderr := runWithEnv(t, root, t.TempDir())
		require.Equal(t, 1, status)
		require.Equal(t, fmt.Sprintf("mkdir -p %s: OK\nchmod 700 %s: Failed\n", keyDir, keyDir), stdout)
		require.Equal(t, "chmod rejected\nsupport/make_necessary_dirs failed\n", stderr)
	})

	t.Run("home and passwd lookups fail", func(t *testing.T) {
		root := t.TempDir()
		writeConfig(t, root, "user: git\n")
		previous := userCurrent
		userCurrent = func() (*user.User, error) { return nil, errors.New("passwd lookup failed") }
		t.Cleanup(func() { userCurrent = previous })
		status, stdout, stderr := runWithEnv(t, root, "")
		require.Equal(t, 1, status)
		require.Empty(t, stdout)
		require.True(t, strings.HasSuffix(stderr, "support/make_necessary_dirs failed\n"))
		require.NoDirExists(t, filepath.Join(root, ".ssh"))
	})
}
