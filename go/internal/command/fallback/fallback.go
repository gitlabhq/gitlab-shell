package fallback

import (
	"os"
	"path/filepath"
	"syscall"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/readwriter"
)

type Command struct {
	RootDir string
	Args    []string
}

var (
	// execFunc is overridden in tests
	execFunc = syscall.Exec
)

const (
	RubyProgram = "gitlab-shell-ruby"
)

func (c *Command) Execute(*readwriter.ReadWriter) error {
	rubyCmd := filepath.Join(c.RootDir, "bin", RubyProgram)

	// Ensure rubyArgs[0] is the full path to gitlab-shell-ruby
	rubyArgs := append([]string{rubyCmd}, c.Args[1:]...)

	return execFunc(rubyCmd, rubyArgs, os.Environ())
}
