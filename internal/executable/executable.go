// Package executable provides utilities for managing and locating executables related to GitLab Shell.
package executable

import (
	"os"
	"path/filepath"
)

// Predefined constants representing various executable names and directories.
const (
	BinDir                    = "bin"
	Healthcheck               = "check"
	GitlabShell               = "gitlab-shell"
	AuthorizedKeysCheck       = "gitlab-shell-authorized-keys-check"
	AuthorizedPrincipalsCheck = "gitlab-shell-authorized-principals-check"
)

// Executable represents an executable binary, including its name and root directory.
type Executable struct {
	Name    string
	RootDir string
}

var (
	// osExecutable is overridden in tests
	osExecutable = os.Executable
)

// New creates a new Executable instance by determining its root directory based on the current executable path.
func New(name string) (*Executable, error) {
	path, err := osExecutable()
	if err != nil {
		return nil, err
	}

	rootDir, err := findRootDir(path)
	if err != nil {
		return nil, err
	}

	executable := &Executable{
		Name:    name,
		RootDir: rootDir,
	}

	return executable, nil
}

func findRootDir(path string) (string, error) {
	// Start: /opt/.../gitlab-shell/bin/gitlab-shell
	// Ends:  /opt/.../gitlab-shell
	rootDir := filepath.Dir(filepath.Dir(path))
	pathFromEnv := os.Getenv("GITLAB_SHELL_DIR")

	if pathFromEnv != "" {
		if _, err := os.Stat(pathFromEnv); os.IsNotExist(err) {
			return "", err
		}

		rootDir = pathFromEnv
	}

	return rootDir, nil
}
