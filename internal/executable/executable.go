package executable

import (
	"os"
	"path/filepath"
)

const (
	BinDir                    = "bin"
	Healthcheck               = "check"
	GitlabShell               = "gitlab-shell"
	AuthorizedKeysCheck       = "gitlab-shell-authorized-keys-check"
	AuthorizedPrincipalsCheck = "gitlab-shell-authorized-principals-check"
)

var (
	// osExecutable is overridden in tests
	osExecutable = os.Executable
)

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
