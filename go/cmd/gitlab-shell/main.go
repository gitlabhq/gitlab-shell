package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/config"
)

// findRootDir determines the root directory (and so, the location of the config
// file) from os.Executable()
func findRootDir() (string, error) {
	if path := os.Getenv("GITLAB_SHELL_DIR"); path != "" {
		return path, nil
	}

	path, err := os.Executable()
	if err != nil {
		return "", err
	}

	// Start: /opt/.../gitlab-shell/bin/gitlab-shell
	// Ends:  /opt/.../gitlab-shell
	return filepath.Dir(filepath.Dir(path)), nil
}

func main() {
	readWriter := &readwriter.ReadWriter{
		Out:    os.Stdout,
		In:     os.Stdin,
		ErrOut: os.Stderr,
	}

	rootDir, err := findRootDir()
	if err != nil {
		fmt.Fprintln(readWriter.ErrOut, "Failed to determine root directory, exiting")
		os.Exit(1)
	}

	config, err := config.NewFromDir(rootDir)
	if err != nil {
		fmt.Fprintln(readWriter.ErrOut, "Failed to read config, exiting")
		os.Exit(1)
	}

	cmd, err := command.New(os.Args, config, readWriter)
	if err != nil {
		// For now this could happen if `SSH_CONNECTION` is not set on
		// the environment
		fmt.Fprintf(readWriter.ErrOut, "%v\n", err)
		os.Exit(1)
	}

	// The command will write to STDOUT on execution or replace the current
	// process in case of the `fallback.Command`
	if err = cmd.Execute(); err != nil {
		fmt.Fprintf(readWriter.ErrOut, "%v\n", err)
		os.Exit(1)
	}
}
