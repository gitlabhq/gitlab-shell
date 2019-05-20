package command

import (
	"fmt"
	"os"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/discover"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/fallback"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/twofactorrecover"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/config"
)

type Command interface {
	Execute(*readwriter.ReadWriter) error
}

func New(arguments []string, config *config.Config) (Command, error) {
	fmt.Printf("------%v---------", os.Environ())

	args, err := commandargs.Parse(arguments)

	if err != nil {
		return nil, err
	}

	if config.FeatureEnabled(string(args.CommandType)) {
		return buildCommand(args, config), nil
	}

	return &fallback.Command{RootDir: config.RootDir, Args: arguments}, nil
}

func buildCommand(args *commandargs.CommandArgs, config *config.Config) Command {
	switch args.CommandType {
	case commandargs.Discover:
		return &discover.Command{Config: config, Args: args}
	case commandargs.TwoFactorRecover:
		return &twofactorrecover.Command{Config: config, Args: args}
	}

	return nil
}
