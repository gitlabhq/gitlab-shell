package command

import (
	"gitlab.com/gitlab-org/gitlab-shell/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/authorizedkeys"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/shared/disallowedcommand"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/executable"
	"gitlab.com/gitlab-org/gitlab-shell/internal/sshenv"
)

func New(e *executable.Executable, arguments []string, env sshenv.Env, config *config.Config, readWriter *readwriter.ReadWriter) (command.Command, error) {
	args, err := Parse(e, arguments, env)
	if err != nil {
		return nil, err
	}

	if cmd := build(args, config, readWriter); cmd != nil {
		return cmd, nil
	}

	return nil, disallowedcommand.Error
}

func Parse(e *executable.Executable, arguments []string, env sshenv.Env) (*commandargs.AuthorizedKeys, error) {
	args := &commandargs.AuthorizedKeys{Arguments: arguments}

	if err := args.Parse(); err != nil {
		return nil, err
	}

	return args, nil
}

func build(args *commandargs.AuthorizedKeys, config *config.Config, readWriter *readwriter.ReadWriter) command.Command {
	return &authorizedkeys.Command{Config: config, Args: args, ReadWriter: readWriter}
}
