package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/config"
)

func migrate(_cfg *config.Config, _args []string) (int, bool) {
	// TODO: decide whether to handle the request in Go or not
	return 0, false
}

// rubyExec will never return. It either replaces the current process with a
// Ruby interpreter, or outputs an error and kills the process.
func execRuby() {
	root := filepath.Dir(os.Args[0])

	rubyCmd := filepath.Join(root, "gitlab-shell-ruby")
	rubyArgs := os.Args[1:]
	rubyEnv := os.Environ()

	execErr := syscall.Exec(rubyCmd, rubyArgs, rubyEnv)
	if execErr != nil {
		fmt.Fprintf(os.Stderr, "Failed to exec(%q): %v\n", rubyCmd, execErr)
		os.Exit(1)
	}
}

func main() {
	// Fall back to Ruby in case of problems reading the config, but issue a
	// warning as this isn't something we can sustain indefinitely
	config, err := config.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to read config, falling back to gitlab-shell-ruby")
		execRuby()
	}

	// Try to handle the command with the Go implementation
	if exitCode, done := migrate(config, os.Args[1:]); done {
		os.Exit(exitCode)
	}

	// Since a migration has not handled the command, fall back to Ruby to do so
	execRuby()
}
