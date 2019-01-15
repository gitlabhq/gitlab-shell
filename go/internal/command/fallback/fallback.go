package fallback

import (
	"os"
	"path/filepath"
	"syscall"
)

type Command struct{}

var (
	binDir = filepath.Dir(os.Args[0])
)

func (c *Command) Execute() error {
	rubyCmd := filepath.Join(binDir, "gitlab-shell-ruby")
	execErr := syscall.Exec(rubyCmd, os.Args, os.Environ())
	return execErr
}
