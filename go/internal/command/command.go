package command

import (
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/discover"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/fallback"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/config"
)

type Command interface {
	Execute() error
}

func New(arguments []string, config *config.Config) (Command, error) {
	args, err := commandargs.Parse(arguments)

	if err != nil {
		return nil, err
	}

	if config.FeatureEnabled(string(args.CommandType)) {
		return buildCommand(args, config), nil
	}

	return &fallback.Command{}, nil
}

func buildCommand(args *commandargs.CommandArgs, config *config.Config) Command {
	switch args.CommandType {
	case commandargs.Discover:
		return &discover.Command{Config: config, Args: args}
	}

	return nil
}
