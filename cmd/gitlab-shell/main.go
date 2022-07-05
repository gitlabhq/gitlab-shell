package main

import (
	"fmt"
	"os"
	"reflect"

	grpccodes "google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	"gitlab.com/gitlab-org/labkit/fips"
	"gitlab.com/gitlab-org/labkit/log"

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
	// We can't use the flag library because gitlab-shell receives other arguments
	// that confuse the parser.
	if len(os.Args) == 2 && os.Args[1] == "-version" {
		fmt.Printf("gitlab-shell %s-%s\n", Version, BuildTime)
		os.Exit(0)
	}

	readWriter := &readwriter.ReadWriter{
		Out:    os.Stdout,
		In:     os.Stdin,
		ErrOut: os.Stderr,
	}

	executable, err := executable.New(executable.GitlabShell)
	if err != nil {
		fmt.Fprintln(readWriter.ErrOut, "Failed to determine executable, exiting")
		os.Exit(1)
	}

	config, err := config.NewFromDirExternal(executable.RootDir)
	if err != nil {
		fmt.Fprintln(readWriter.ErrOut, "Failed to read config, exiting")
		os.Exit(1)
	}

	logCloser := logger.Configure(config)
	defer logCloser.Close()

	env := sshenv.NewFromEnv()
	cmd, err := shellCmd.New(os.Args[1:], env, config, readWriter)
	if err != nil {
		// For now this could happen if `SSH_CONNECTION` is not set on
		// the environment
		fmt.Fprintf(readWriter.ErrOut, "%v\n", err)
		os.Exit(1)
	}

	ctx, finished := command.Setup(executable.Name, config)
	defer finished()

	config.GitalyClient.InitSidechannelRegistry(ctx)

	cmdName := reflect.TypeOf(cmd).String()
	ctxlog := log.ContextLogger(ctx)
	ctxlog.WithFields(log.Fields{"env": env, "command": cmdName}).Info("gitlab-shell: main: executing command")
	fips.Check()

	if err := cmd.Execute(ctx); err != nil {
		ctxlog.WithError(err).Warn("gitlab-shell: main: command execution failed")
		if grpcstatus.Convert(err).Code() != grpccodes.Internal {
			console.DisplayWarningMessage(err.Error(), readWriter.ErrOut)
		}
		os.Exit(1)
	}

	ctxlog.Info("gitlab-shell: main: command executed successfully")
}
