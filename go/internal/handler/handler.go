package handler

import (
	"os"
	"os/exec"
	"syscall"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/logger"
)

func Prepare() error {
	cfg, err := config.New()
	if err != nil {
		return err
	}

	if err := logger.Configure(cfg); err != nil {
		return err
	}

	// Use a working directory that won't get removed or unmounted.
	if err := os.Chdir("/"); err != nil {
		return err
	}

	return nil
}

func execCommand(command string, args ...string) error {
	binPath, err := exec.LookPath(command)
	if err != nil {
		return err
	}

	args = append([]string{binPath}, args...)
	return syscall.Exec(binPath, args, os.Environ())
}
