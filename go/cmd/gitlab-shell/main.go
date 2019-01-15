package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/fallback"
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

// rubyExec will never return. It either replaces the current process with a
// Ruby interpreter, or outputs an error and kills the process.
func execRuby() {
	cmd := &fallback.Command{}
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to exec: %v\n", err)
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

	cmd, err := command.New(os.Args, config)
	if err != nil {
		// For now this could happen if `SSH_CONNECTION` is not set on
		// the environment
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	// The command will write to STDOUT on execution or replace the current
	// process in case of the `fallback.Command`
	if err = cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
