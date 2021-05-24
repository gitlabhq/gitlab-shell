package main

import (
	"fmt"
	"os"

	"gitlab.com/gitlab-org/gitlab-shell/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/console"
	"gitlab.com/gitlab-org/gitlab-shell/internal/executable"
	"gitlab.com/gitlab-org/gitlab-shell/internal/logger"
	"gitlab.com/gitlab-org/gitlab-shell/internal/sshenv"
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

	logger.Configure(config)

	env := sshenv.NewFromEnv()
	cmd, err := command.New(executable, os.Args[1:], env, config, readWriter)
	if err != nil {
		// For now this could happen if `SSH_CONNECTION` is not set on
		// the environment
		fmt.Fprintf(readWriter.ErrOut, "%v\n", err)
		os.Exit(1)
	}

	ctx, finished := command.Setup(executable.Name, config)
	defer finished()

	if err = cmd.Execute(ctx); err != nil {
		console.DisplayWarningMessage(err.Error(), readWriter.ErrOut)
		os.Exit(1)
	}
}
