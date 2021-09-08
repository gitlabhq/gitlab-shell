package command

import (
	"context"

	"gitlab.com/gitlab-org/gitlab-shell/internal/command/authorizedkeys"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/authorizedprincipals"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/discover"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/healthcheck"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/lfsauthenticate"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/personalaccesstoken"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/receivepack"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/shared/disallowedcommand"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/twofactorrecover"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/twofactorverify"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/uploadarchive"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/uploadpack"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/executable"
	"gitlab.com/gitlab-org/gitlab-shell/internal/sshenv"
	"gitlab.com/gitlab-org/labkit/correlation"
	"gitlab.com/gitlab-org/labkit/tracing"
)

type Command interface {
	Execute(ctx context.Context) error
}

func New(e *executable.Executable, arguments []string, env sshenv.Env, config *config.Config, readWriter *readwriter.ReadWriter) (Command, error) {
	var args commandargs.CommandArgs
	if e.AcceptArgs {
		var err error
		args, err = commandargs.Parse(e, arguments, env)
		if err != nil {
			return nil, err
		}
	}

	if cmd := buildCommand(e, args, config, readWriter); cmd != nil {
		return cmd, nil
	}

	return nil, disallowedcommand.Error
}

// Setup() initializes tracing from the configuration file and generates a
// background context from which all other contexts in the process should derive
// from, as it has a service name and initial correlation ID set.
func Setup(serviceName string, config *config.Config) (context.Context, func()) {
	closer := tracing.Initialize(
		tracing.WithServiceName(serviceName),

		// For GitLab-Shell, we explicitly initialize tracing from a config file
		// instead of the default environment variable (using GITLAB_TRACING)
		// This decision was made owing to the difficulty in passing environment
		// variables into GitLab-Shell processes.
		//
		// Processes are spawned as children of the SSH daemon, which tightly
		// controls environment variables; doing this means we don't have to
		// enable PermitUserEnvironment
		//
		// gitlab-sshd could use the standard GITLAB_TRACING envvar, but that
		// would lead to inconsistencies between the two forms of operation
		tracing.WithConnectionString(config.GitlabTracing),
	)

	ctx, finished := tracing.ExtractFromEnv(context.Background())
	ctx = correlation.ContextWithClientName(ctx, serviceName)

	correlationID := correlation.ExtractFromContext(ctx)
	if correlationID == "" {
		correlationID := correlation.SafeRandomID()
		ctx = correlation.ContextWithCorrelation(ctx, correlationID)
	}

	return ctx, func() {
		finished()
		closer.Close()
	}
}

func buildCommand(e *executable.Executable, args commandargs.CommandArgs, config *config.Config, readWriter *readwriter.ReadWriter) Command {
	switch e.Name {
	case executable.GitlabShell:
		return BuildShellCommand(args.(*commandargs.Shell), config, readWriter)
	case executable.AuthorizedKeysCheck:
		return buildAuthorizedKeysCommand(args.(*commandargs.AuthorizedKeys), config, readWriter)
	case executable.AuthorizedPrincipalsCheck:
		return buildAuthorizedPrincipalsCommand(args.(*commandargs.AuthorizedPrincipals), config, readWriter)
	case executable.Healthcheck:
		return buildHealthcheckCommand(config, readWriter)
	}

	return nil
}

func BuildShellCommand(args *commandargs.Shell, config *config.Config, readWriter *readwriter.ReadWriter) Command {
	switch args.CommandType {
	case commandargs.Discover:
		return &discover.Command{Config: config, Args: args, ReadWriter: readWriter}
	case commandargs.TwoFactorRecover:
		return &twofactorrecover.Command{Config: config, Args: args, ReadWriter: readWriter}
	case commandargs.TwoFactorVerify:
		return &twofactorverify.Command{Config: config, Args: args, ReadWriter: readWriter}
	case commandargs.LfsAuthenticate:
		return &lfsauthenticate.Command{Config: config, Args: args, ReadWriter: readWriter}
	case commandargs.ReceivePack:
		return &receivepack.Command{Config: config, Args: args, ReadWriter: readWriter}
	case commandargs.UploadPack:
		return &uploadpack.Command{Config: config, Args: args, ReadWriter: readWriter}
	case commandargs.UploadArchive:
		return &uploadarchive.Command{Config: config, Args: args, ReadWriter: readWriter}
	case commandargs.PersonalAccessToken:
		return &personalaccesstoken.Command{Config: config, Args: args, ReadWriter: readWriter}
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
