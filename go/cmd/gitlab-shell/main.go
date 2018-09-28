package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/config"
)

var (
	binDir  string
	rootDir string
)

func init() {
	binDir = filepath.Dir(os.Args[0])
	rootDir = filepath.Dir(binDir)
}

func migrate(*config.Config) (int, bool) {
	// TODO: Dispatch appropriate requests to Go handlers and return
	// <exitstatus, true> depending on how they fare
	return 0, false
}

// rubyExec will never return. It either replaces the current process with a
// Ruby interpreter, or outputs an error and kills the process.
func execRuby() {
	rubyCmd := filepath.Join(binDir, "gitlab-shell-ruby")

	execErr := syscall.Exec(rubyCmd, os.Args, os.Environ())
	if execErr != nil {
		fmt.Fprintf(os.Stderr, "Failed to exec(%q): %v\n", rubyCmd, execErr)
		os.Exit(1)
	}
}

func main() {
	// Fall back to Ruby in case of problems reading the config, but issue a
	// warning as this isn't something we can sustain indefinitely
	config, err := config.NewFromDir(rootDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to read config, falling back to gitlab-shell-ruby")
		execRuby()
	}

	// Try to handle the command with the Go implementation
	if exitCode, done := migrate(config); done {
		os.Exit(exitCode)
	}

	// Since a migration has not handled the command, fall back to Ruby to do so
	execRuby()
}
