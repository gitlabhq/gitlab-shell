package fallback

import (
	"os"
	"path/filepath"
	"syscall"
)

type Executable struct {
	Program string
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

func (e *Executable) Execute() error {
	if e.Program == "" {
		e.Program = RubyProgram
	}

	rubyCmd := filepath.Join(e.RootDir, "bin", e.Program)

	// Ensure rubyArgs[0] is the full path to Ruby program
	rubyArgs := append([]string{rubyCmd}, e.Args[1:]...)

	return execFunc(rubyCmd, rubyArgs, os.Environ())
}
