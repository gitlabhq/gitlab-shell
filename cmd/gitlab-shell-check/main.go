// Package main is the entry point for the GitLab Shell health check command.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	checkCmd "gitlab.com/gitlab-org/gitlab-shell/v14/cmd/gitlab-shell-check/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/executable"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/logger"
	"gitlab.com/gitlab-org/labkit/fields"
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
	ctx := context.Background()
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

	log, logCloser, err := logger.ConfigureLogger(config)
	if err != nil {
		log.ErrorContext(ctx, "failed to log to file, reverting to stderr", slog.String(fields.ErrorMessage, err.Error()))
	} else {
		// nolint
		defer func() {
			if err = logCloser.Close(); err != nil {
				log.ErrorContext(ctx, "failed to close log file", slog.String(fields.ErrorMessage, err.Error()))
			}
		}()
	}
	slog.SetDefault(log)

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
