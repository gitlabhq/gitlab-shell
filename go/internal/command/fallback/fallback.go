package fallback

import (
	"os"
	"path/filepath"
	"syscall"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/readwriter"
)

type Command struct{}

var (
	binDir = filepath.Dir(os.Args[0])
)

func (c *Command) Execute(_ *readwriter.ReadWriter) error {
	rubyCmd := filepath.Join(binDir, "gitlab-shell-ruby")
	execErr := syscall.Exec(rubyCmd, os.Args, os.Environ())
	return execErr
}
