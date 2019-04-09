package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/fallback"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/config"
)

// findRootDir determines the root directory (and so, the location of the config
// file) from os.Executable()
func findRootDir() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", err
	}

	// Start: /opt/.../gitlab-shell/bin/gitlab-shell
	// Ends:  /opt/.../gitlab-shell
	return filepath.Dir(filepath.Dir(path)), nil
}

// rubyExec will never return. It either replaces the current process with a
// Ruby interpreter, or outputs an error and kills the process.
func execRuby(readWriter *readwriter.ReadWriter) {
	cmd := &fallback.Command{}
	if err := cmd.Execute(readWriter); err != nil {
		fmt.Fprintf(readWriter.ErrOut, "Failed to exec: %v\n", err)
		os.Exit(1)
	}
}

func main() {
	readWriter := &readwriter.ReadWriter{
		Out:    os.Stdout,
		In:     os.Stdin,
		ErrOut: os.Stderr,
	}

	rootDir, err := findRootDir()
	if err != nil {
		fmt.Fprintln(readWriter.ErrOut, "Failed to determine root directory, falling back to gitlab-shell-ruby")
		execRuby(readWriter)
	}

	// Fall back to Ruby in case of problems reading the config, but issue a
	// warning as this isn't something we can sustain indefinitely
	config, err := config.NewFromDir(rootDir)
	if err != nil {
		fmt.Fprintln(readWriter.ErrOut, "Failed to read config, falling back to gitlab-shell-ruby")
		execRuby(readWriter)
	}

	cmd, err := command.New(os.Args, config)
	if err != nil {
		// For now this could happen if `SSH_CONNECTION` is not set on
		// the environment
		fmt.Fprintf(readWriter.ErrOut, "%v\n", err)
		os.Exit(1)
	}

	// The command will write to STDOUT on execution or replace the current
	// process in case of the `fallback.Command`
	if err = cmd.Execute(readWriter); err != nil {
		fmt.Fprintf(readWriter.ErrOut, "%v\n", err)
		os.Exit(1)
	}
}
