package fallback

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/commandargs"
)

type Command struct {
	RootDir string
	Args    commandargs.CommandArgs
}

var (
	// execFunc is overridden in tests
	execFunc = syscall.Exec
)

func (c *Command) Execute() error {
	rubyCmd := filepath.Join(c.RootDir, "bin", c.fallbackProgram())

	// Ensure rubyArgs[0] is the full path to gitlab-shell-ruby
	rubyArgs := append([]string{rubyCmd}, c.Args.Arguments()...)

	return execFunc(rubyCmd, rubyArgs, os.Environ())
}

func (c *Command) fallbackProgram() string {
	return fmt.Sprintf("%s-ruby", c.Args.Executable())
}
