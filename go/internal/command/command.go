package command

import (
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/discover"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/fallback"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/receivepack"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/twofactorrecover"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/config"
)

type Command interface {
	Execute() error
}

func New(arguments []string, config *config.Config, readWriter *readwriter.ReadWriter) (Command, error) {
	args, err := commandargs.Parse(arguments)

	if err != nil {
		return nil, err
	}

	if config.FeatureEnabled(string(args.CommandType)) {
		return buildCommand(args, config, readWriter), nil
	}

	return &fallback.Command{RootDir: config.RootDir, Args: arguments}, nil
}

func buildCommand(args *commandargs.CommandArgs, config *config.Config, readWriter *readwriter.ReadWriter) Command {
	switch args.CommandType {
	case commandargs.Discover:
		return &discover.Command{Config: config, Args: args, ReadWriter: readWriter}
	case commandargs.TwoFactorRecover:
		return &twofactorrecover.Command{Config: config, Args: args, ReadWriter: readWriter}
	case commandargs.ReceivePack:
		return &receivepack.Command{Config: config, Args: args, ReadWriter: readWriter}
	}

	return nil
}
