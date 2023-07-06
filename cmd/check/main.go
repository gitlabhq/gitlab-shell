package main

import (
	"fmt"
	"os"
	"path"

	checkCmd "gitlab.com/gitlab-org/gitlab-shell/v14/cmd/check/command"
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
	// We can't use the flag library because gitlab-shell receives other arguments
	// that confuse the parser.
	if len(os.Args) == 2 && os.Args[1] == "-version" {
		fmt.Printf("%s %s-%s\n", path.Base(os.Args[0]), Version, BuildTime)
		os.Exit(0)
	}

	readWriter := &readwriter.ReadWriter{
		Out:    os.Stdout,
		In:     os.Stdin,
		ErrOut: os.Stderr,
	}

	executable, err := executable.New(executable.Healthcheck)
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

	cmd, err := checkCmd.New(config, readWriter)
	if err != nil {
		fmt.Fprintf(readWriter.ErrOut, "%v\n", err)
		os.Exit(1)
	}

	ctx, finished := command.Setup(executable.Name, config)
	defer finished()

	if ctx, err = cmd.Execute(ctx); err != nil {
		fmt.Fprintf(readWriter.ErrOut, "%v\n", err)
		os.Exit(1)
	}
}
