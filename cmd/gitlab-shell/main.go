// Package main is the entry point for the gitlab-shell command
package main

import (
	"fmt"
	"log/slog"
	"os"
	"reflect"

	grpccodes "google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	"gitlab.com/gitlab-org/labkit/fields"
	"gitlab.com/gitlab-org/labkit/fips"

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

func main() {
	command.CheckForVersionFlag(os.Args, Version, BuildTime)

	readWriter := &readwriter.ReadWriter{
		Out:    &readwriter.CountingWriter{W: os.Stdout},
		In:     os.Stdin,
		ErrOut: os.Stderr,
	}

	executable, err := executable.New(executable.GitlabShell)
	if err != nil {
		_, _ = fmt.Fprintln(readWriter.ErrOut, "Failed to determine executable, exiting")
		os.Exit(1)
	}

	config, err := config.NewFromDirExternal(executable.RootDir)
	if err != nil {
		_, _ = fmt.Fprintln(readWriter.ErrOut, "Failed to read config, exiting:", err)
		os.Exit(1)
	}

	logger, logCloser, err := logger.ConfigureLogger(config)
	if err != nil && logCloser != nil {
		logger.Error("Failed to configure logging", slog.String(fields.ErrorMessage, err.Error()))
	}
	slog.SetDefault(logger)

	env := sshenv.NewFromEnv()
	cmd, err := shellCmd.New(os.Args[1:], env, config, readWriter)
	if err != nil {
		// For now this could happen if `SSH_CONNECTION` is not set on
		// the environment
		_, _ = fmt.Fprintf(readWriter.ErrOut, "%v\n", err)
		_ = logCloser.Close()
		os.Exit(1)
	}

	ctx, finished := command.Setup(executable.Name, config)

	config.GitalyClient.InitSidechannelRegistry(ctx)

	cmdName := reflect.TypeOf(cmd).String()
	logger.InfoContext(ctx, "gitlab-shell: main: executing command", slog.Any("env", env), slog.String("command", cmdName))
	fips.Check()

	if _, err := cmd.Execute(ctx); err != nil {
		logger.WarnContext(ctx, "gitlab-shell: main: command execution failed", slog.String(fields.ErrorMessage, err.Error()))
		if grpcstatus.Convert(err).Code() != grpccodes.Internal {
			console.DisplayWarningMessage(err.Error(), readWriter.ErrOut)
		}
		finished()
		_ = logCloser.Close()
		os.Exit(1)
	}

	logger.InfoContext(ctx, "gitlab-shell: main: command executed successfully")
	finished()
	_ = logCloser.Close()
}
