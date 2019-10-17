package command

import (
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/authorizedkeys"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/authorizedprincipals"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/discover"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/healthcheck"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/lfsauthenticate"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/receivepack"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/shared/disallowedcommand"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/twofactorrecover"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/uploadarchive"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/uploadpack"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/executable"
)

type Command interface {
	Execute() error
}

func New(e *executable.Executable, arguments []string, config *config.Config, readWriter *readwriter.ReadWriter) (Command, error) {
	args, err := commandargs.Parse(e, arguments)
	if err != nil {
		return nil, err
	}

	if cmd := buildCommand(e, args, config, readWriter); cmd != nil {
		return cmd, nil
	}

	return nil, disallowedcommand.Error
}

func buildCommand(e *executable.Executable, args commandargs.CommandArgs, config *config.Config, readWriter *readwriter.ReadWriter) Command {
	switch e.Name {
	case executable.GitlabShell:
		return buildShellCommand(args.(*commandargs.Shell), config, readWriter)
	case executable.AuthorizedKeysCheck:
		return buildAuthorizedKeysCommand(args.(*commandargs.AuthorizedKeys), config, readWriter)
	case executable.AuthorizedPrincipalsCheck:
		return buildAuthorizedPrincipalsCommand(args.(*commandargs.AuthorizedPrincipals), config, readWriter)
	case executable.Healthcheck:
		return buildHealthcheckCommand(config, readWriter)
	}

	return nil
}

func buildShellCommand(args *commandargs.Shell, config *config.Config, readWriter *readwriter.ReadWriter) Command {
	switch args.CommandType {
	case commandargs.Discover:
		return &discover.Command{Config: config, Args: args, ReadWriter: readWriter}
	case commandargs.TwoFactorRecover:
		return &twofactorrecover.Command{Config: config, Args: args, ReadWriter: readWriter}
	case commandargs.LfsAuthenticate:
		return &lfsauthenticate.Command{Config: config, Args: args, ReadWriter: readWriter}
	case commandargs.ReceivePack:
		return &receivepack.Command{Config: config, Args: args, ReadWriter: readWriter}
	case commandargs.UploadPack:
		return &uploadpack.Command{Config: config, Args: args, ReadWriter: readWriter}
	case commandargs.UploadArchive:
		return &uploadarchive.Command{Config: config, Args: args, ReadWriter: readWriter}
	}

	return nil
}

func buildAuthorizedKeysCommand(args *commandargs.AuthorizedKeys, config *config.Config, readWriter *readwriter.ReadWriter) Command {
	return &authorizedkeys.Command{Config: config, Args: args, ReadWriter: readWriter}
}

func buildAuthorizedPrincipalsCommand(args *commandargs.AuthorizedPrincipals, config *config.Config, readWriter *readwriter.ReadWriter) Command {
	return &authorizedprincipals.Command{Config: config, Args: args, ReadWriter: readWriter}
}

func buildHealthcheckCommand(config *config.Config, readWriter *readwriter.ReadWriter) Command {
	return &healthcheck.Command{Config: config, ReadWriter: readWriter}
}
