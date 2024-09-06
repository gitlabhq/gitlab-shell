// Package command handles command creation and initialization in GitLab Shell.
package command

import (
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/authorizedprincipals"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/disallowedcommand"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
)

// New creates a new command based on provided arguments, config, and I/O.
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

// Parse parses command-line arguments into a CommandArgs structure.
func Parse(arguments []string) (*commandargs.AuthorizedPrincipals, error) {
	args := &commandargs.AuthorizedPrincipals{Arguments: arguments}

	if err := args.Parse(); err != nil {
		return nil, err
	}

	return args, nil
}

func build(args *commandargs.AuthorizedPrincipals, config *config.Config, readWriter *readwriter.ReadWriter) command.Command {
	return &authorizedprincipals.Command{Config: config, Args: args, ReadWriter: readWriter}
}
