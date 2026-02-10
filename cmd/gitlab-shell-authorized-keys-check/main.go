// Package main is the entry point for the gitlab-shell-authorized-keys-check command
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	cmd "gitlab.com/gitlab-org/gitlab-shell/v14/cmd/gitlab-shell-authorized-keys-check/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/console"
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

const (
	exitCodeSuccess = 0
	exitCodeFailure = 1
)

func main() {
	command.CheckForVersionFlag(os.Args, Version, BuildTime)

	readWriter := &readwriter.ReadWriter{
		Out:    &readwriter.CountingWriter{W: os.Stdout},
		In:     os.Stdin,
		ErrOut: os.Stderr,
	}

	code, err := execute(readWriter)
	if err != nil {
		_, _ = fmt.Fprintf(readWriter.ErrOut, "%v\n", err)
	}

	os.Exit(code)
}

func execute(readWriter *readwriter.ReadWriter) (int, error) {
	ctx := context.Background()
	executable, err := executable.New(executable.AuthorizedKeysCheck)
	if err != nil {
		return exitCodeFailure, fmt.Errorf("failed to determine executable, exiting")
	}

	config, err := config.NewFromDirExternal(executable.RootDir)
	if err != nil {
		return exitCodeFailure, fmt.Errorf("failed to read config, exiting")
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

	cmd, err := cmd.New(os.Args[1:], config, readWriter)
	if err != nil {
		// For now this could happen if `SSH_CONNECTION` is not set on
		// the environment
		return exitCodeFailure, err
	}

	ctx, finished := command.Setup(executable.Name, config)
	defer finished()

	if _, err = cmd.Execute(ctx); err != nil {
		console.DisplayWarningMessage(err.Error(), readWriter.ErrOut)
		return exitCodeFailure, nil
	}

	return exitCodeSuccess, nil
}
