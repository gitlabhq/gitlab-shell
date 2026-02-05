// Package main is the entry point for the gitlab-shell command
package main

import (
	"fmt"
	"os"
	"reflect"

	grpccodes "google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	"gitlab.com/gitlab-org/labkit/fips"
	"gitlab.com/gitlab-org/labkit/log"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	shellCmd "gitlab.com/gitlab-org/gitlab-shell/v14/cmd/gitlab-shell/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/console"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/executable"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/logger"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/metrics"
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

	logCloser := logger.Configure(logger.LogOptions{
		LogFile:  config.LogFile,
		LogFmt:   config.LogFormat,
		LogLevel: config.LogLevel,
	})

	client, err := client.NewHTTPClientWithOpts(
		config.GitlabURL,
		config.GitlabRelativeURLRoot,
		config.HTTPSettings.CaFile,
		config.HTTPSettings.CaPath,
		config.HTTPSettings.ReadTimeoutSeconds,
		nil,
	)
	if err != nil {
		_, _ = fmt.Fprintln(readWriter.ErrOut, "Failed to create http client, exiting:", err)
		os.Exit(1)
	}

	tr := client.RetryableHTTP.HTTPClient.Transport
	client.RetryableHTTP.HTTPClient.Transport = metrics.NewRoundTripper(tr)

	env := sshenv.NewFromEnv()
	cmd, err := shellCmd.New(os.Args[1:], env, config, readWriter, client)
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
	ctxlog := log.ContextLogger(ctx)
	ctxlog.WithFields(log.Fields{"env": env, "command": cmdName}).Info("gitlab-shell: main: executing command")
	fips.Check()

	if _, err := cmd.Execute(ctx); err != nil {
		ctxlog.WithError(err).Warn("gitlab-shell: main: command execution failed")
		if grpcstatus.Convert(err).Code() != grpccodes.Internal {
			console.DisplayWarningMessage(err.Error(), readWriter.ErrOut)
		}
		finished()
		_ = logCloser.Close()
		os.Exit(1)
	}

	ctxlog.Info("gitlab-shell: main: command executed successfully")
	finished()
	_ = logCloser.Close()
}
