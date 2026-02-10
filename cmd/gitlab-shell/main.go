// Package main is the entry point for the gitlab-shell command
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"reflect"

	grpccodes "google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	"gitlab.com/gitlab-org/labkit/correlation"
	"gitlab.com/gitlab-org/labkit/fields"
	"gitlab.com/gitlab-org/labkit/fips"
	"gitlab.com/gitlab-org/labkit/v2/log"

	shellCmd "gitlab.com/gitlab-org/gitlab-shell/v14/cmd/gitlab-shell/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/console"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/executable"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/logger"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/sshenv"
)

var (
	// Version is the current version of gitlab-shell
	Version = "(unknown version)" // Set at build time in the Makefile
	// BuildTime signifies the time the binary was build
	BuildTime = "19700101.000000" // Set at build time in the Makefile
)

// nolint
func run() error {
	ctx := context.Background()
	command.CheckForVersionFlag(os.Args, Version, BuildTime)

	readWriter := &readwriter.ReadWriter{
		Out:    &readwriter.CountingWriter{W: os.Stdout},
		In:     os.Stdin,
		ErrOut: os.Stderr,
	}

	executable, err := executable.New(executable.GitlabShell)
	if err != nil {
		_, _ = fmt.Fprintln(readWriter.ErrOut, "Failed to determine executable, exiting")
		return err
	}

	config, err := config.NewFromDirExternal(executable.RootDir)
	if err != nil {
		_, _ = fmt.Fprintln(readWriter.ErrOut, "Failed to read config, exiting:", err)
		return err
	}

	logger, logCloser, err := logger.ConfigureLogger(config)
	if err != nil {
		logger.ErrorContext(ctx, "failed to log to file, reverting to stderr", slog.String(fields.ErrorMessage, err.Error()))
	} else {
		// nolint
		defer func() {
			if err = logCloser.Close(); err != nil {
				logger.ErrorContext(ctx, "failed to close log file", slog.String(fields.ErrorMessage, err.Error()))
			}
		}()
	}
	slog.SetDefault(logger)

	env := sshenv.NewFromEnv()
	cmd, err := shellCmd.New(os.Args[1:], env, config, readWriter)
	if err != nil {
		// For now this could happen if `SSH_CONNECTION` is not set on
		// the environment
		_, _ = fmt.Fprintf(readWriter.ErrOut, "%v\n", err)
		return err
	}

	ctx, finished := command.Setup(executable.Name, config)

	config.GitalyClient.InitSidechannelRegistry(ctx)

	cmdName := reflect.TypeOf(cmd).String()
	ctx = log.WithFields(ctx,
		slog.String(fields.CorrelationID, correlation.ExtractFromContext(ctx)),
		slog.Any("env", env),
		slog.String("command", cmdName),
	)

	logger.InfoContext(ctx, "gitlab-shell: main: executing command")
	fips.Check()

	if _, err := cmd.Execute(ctx); err != nil {
		logger.WarnContext(ctx, "gitlab-shell: main: command execution failed", slog.String(fields.ErrorMessage, err.Error()))
		if grpcstatus.Convert(err).Code() != grpccodes.Internal {
			console.DisplayWarningMessage(err.Error(), readWriter.ErrOut)
		}
		finished()
		return err
	}

	logger.InfoContext(ctx, "gitlab-shell: main: command executed successfully")
	finished()
	return nil
}

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}
