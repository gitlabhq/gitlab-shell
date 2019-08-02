package fallback

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/executable"
)

type Command struct {
	Executable *executable.Executable
	RootDir    string
	Args       commandargs.CommandArgs
}

var (
	// execFunc is overridden in tests
	execFunc  = syscall.Exec
	whitelist = []string{
		executable.GitlabShell,
		executable.AuthorizedKeysCheck,
		executable.AuthorizedPrincipalsCheck,
	}
)

func (c *Command) Execute() error {
	if !c.isWhitelisted() {
		return errors.New("Failed to execute unknown executable")
	}

	rubyCmd := c.fallbackProgram()

	// Ensure rubyArgs[0] is the full path to gitlab-shell-ruby
	rubyArgs := append([]string{rubyCmd}, c.Args.GetArguments()...)

	return execFunc(rubyCmd, rubyArgs, os.Environ())
}

func (c *Command) isWhitelisted() bool {
	for _, item := range whitelist {
		if c.Executable.Name == item {
			return true
		}
	}

	return false
}

func (c *Command) fallbackProgram() string {
	fileName := fmt.Sprintf("%s-ruby", c.Executable.Name)

	return filepath.Join(c.RootDir, "bin", fileName)
}
