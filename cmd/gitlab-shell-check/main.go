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

	exitOnError := func(err error, message string) int {
		if err != nil {
			_, _ = fmt.Fprintf(readWriter.ErrOut, "%s: %v\n", message, err)
			return 1
		}
		return 0
	}

	executable, err := executable.New(executable.Healthcheck)
	if code := exitOnError(err, "Failed to determine executable, exiting"); code != 0 {
		return code
	}

	config, err := config.NewFromDirExternal(executable.RootDir)
	if code := exitOnError(err, "Failed to read config, exiting"); code != 0 {
		return code
	}

	logCloser := logger.Configure(config)
	defer logCloser.Close() //nolint:errcheck

	cmd, err := checkCmd.New(config, readWriter)
	if code := exitOnError(err, "Failed to create command"); code != 0 {
		return code
	}

	ctx, finished := command.Setup(executable.Name, config)
	defer finished()

	if _, err = cmd.Execute(ctx); err != nil {
		_, _ = fmt.Fprintf(readWriter.ErrOut, "%v\n", err)
		return 1
	}
	return 0
}
