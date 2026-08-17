// Package main is the entry point for the GitLab Shell health check command.
package main

import (
	"fmt"
	"os"

	checkCmd "gitlab.com/gitlab-org/gitlab-shell/v14/cmd/gitlab-shell-check/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/executable"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/logger"
)

var (
	// Version is the current version of gitlab-shell
	Version = "(unknown version)" // Set at build time in the Makefile
	// BuildTime signifies the time the binary was build
	BuildTime = "19700101.000000" // Set at build time in the Makefile
)

func main() {
	os.Exit(run())
}

func run() int {
	command.CheckForVersionFlag(os.Args, Version, BuildTime)

	readWriter := &readwriter.ReadWriter{
		Out:    &readwriter.CountingWriter{W: os.Stdout},
		In:     os.Stdin,
		ErrOut: os.Stderr,
	}

	executable, err := executable.New(executable.Healthcheck)
	if err != nil {
		_, _ = fmt.Fprintf(readWriter.ErrOut, "Failed to determine executable, exiting: %v\n", err)
		return 1
	}

	config, err := config.NewFromDirExternal(executable.RootDir)
	if err != nil {
		_, _ = fmt.Fprintf(readWriter.ErrOut, "Failed to read config, exiting: %v\n", err)
		return 1
	}
	defer config.Close() //nolint:errcheck

	logCloser := logger.ConfigureLogger(config)
	if logCloser != nil {
		defer logCloser.Close() //nolint:errcheck
	}

	cmd, err := checkCmd.New(config, readWriter)
	if err != nil {
		_, _ = fmt.Fprintf(readWriter.ErrOut, "Failed to create command: %v\n", err)
		return 1
	}

	ctx, finished := command.Setup(executable.Name, config)
	defer finished()

	if _, err = cmd.Execute(ctx); err != nil {
		_, _ = fmt.Fprintf(readWriter.ErrOut, "%v\n", err)
		return 1
	}
	return 0
}
