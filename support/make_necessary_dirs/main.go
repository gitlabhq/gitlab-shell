// Package main implements support/make_necessary_dirs, run by `make make_necessary_dirs`
// during GitLab's gitlab:shell:install task. It creates the directory containing
// the configured auth_file and sets its mode to 0700.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	programName    = "support/make_necessary_dirs"
	configFileName = "config.yml"
)

var chmod = os.Chmod
var userCurrent = user.Current

func main() {
	os.Exit(run(os.Stdout, os.Stderr))
}

func run(stdout, stderr io.Writer) int {
	keyDir, err := resolveKeyDir()
	if err != nil {
		return fail(stderr, err)
	}

	_, _ = fmt.Fprintf(stdout, "mkdir -p %s: ", keyDir)
	if err := os.MkdirAll(keyDir, 0o777); err != nil { //nolint:gosec // Match mkdir -p defaults before restricting the key directory with chmod.
		_, _ = fmt.Fprintln(stdout, "Failed")
		return fail(stderr, err)
	}
	_, _ = fmt.Fprintln(stdout, "OK")

	_, _ = fmt.Fprintf(stdout, "chmod 700 %s: ", keyDir)
	if err := chmod(keyDir, 0o700); err != nil {
		_, _ = fmt.Fprintln(stdout, "Failed")
		return fail(stderr, err)
	}
	_, _ = fmt.Fprintln(stdout, "OK")
	return 0
}

func fail(stderr io.Writer, err error) int {
	_, _ = fmt.Fprintln(stderr, err)
	_, _ = fmt.Fprintf(stderr, "%s failed\n", programName)
	return 1
}

func resolveKeyDir() (string, error) {
	root, err := rootDir()
	if err != nil {
		return "", err
	}
	// Mirror Ruby's File.join without filepath.Join: cleaning .. before a symlink changes filesystem traversal.
	authFile, err := readAuthFile(strings.TrimRight(root, "/") + "/" + configFileName)
	if err != nil {
		return "", err
	}
	if authFile == "" {
		home, err := homeDir()
		if err != nil {
			return "", err
		}
		authFile = filepath.Join(home, ".ssh", "authorized_keys")
	}
	return keyDirectory(authFile), nil
}

func homeDir() (string, error) {
	if home, err := os.UserHomeDir(); err == nil {
		return home, nil
	}
	u, err := userCurrent()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	if u.HomeDir == "" {
		return "", errors.New("resolve home directory: passwd home directory is empty")
	}
	return u.HomeDir, nil
}

// Mirror Ruby's File.dirname without filepath.Clean: .. after a symlink must resolve through
// the filesystem during mkdir -p, so filepath.Dir must not replace this function.
func keyDirectory(authFile string) string {
	if authFile == "" {
		return "."
	}
	authFile = strings.TrimRight(authFile, "/")
	if authFile == "" {
		return "/"
	}
	separator := strings.LastIndex(authFile, "/")
	if separator == -1 {
		return "."
	}
	parent := strings.TrimRight(authFile[:separator], "/")
	if parent == "" {
		return "/"
	}
	return parent
}

func rootDir() (string, error) {
	if root := os.Getenv("GITLAB_SHELL_DIR"); root != "" {
		return root, nil
	}
	return os.Getwd()
}

func readAuthFile(path string) (string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // The config path is selected by the installer or GITLAB_SHELL_DIR.
	if err != nil {
		return "", err
	}
	var config struct {
		AuthFile string `yaml:"auth_file"`
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return "", fmt.Errorf("parse %s: %w", path, err)
	}
	return config.AuthFile, nil
}
