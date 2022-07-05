package command

import (
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/authorizedkeys"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/disallowedcommand"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
)

func New(arguments []string, config *config.Config, readWriter *readwriter.ReadWriter) (command.Command, error) {
	args, err := Parse(arguments)
	if err != nil {
		return nil, err
	}

	if cmd := build(args, config, readWriter); cmd != nil {
		return cmd, nil
	}

	return nil, disallowedcommand.Error
}

func Parse(arguments []string) (*commandargs.AuthorizedKeys, error) {
	args := &commandargs.AuthorizedKeys{Arguments: arguments}

	if err := args.Parse(); err != nil {
		return nil, err
	}

	return args, nil
}

func build(args *commandargs.AuthorizedKeys, config *config.Config, readWriter *readwriter.ReadWriter) command.Command {
	return &authorizedkeys.Command{Config: config, Args: args, ReadWriter: readWriter}
}
