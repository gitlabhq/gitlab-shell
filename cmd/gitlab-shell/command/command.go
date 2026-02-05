// Package command provides functionality for handling GitLab Shell commands
package command

import (
	"slices"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/discover"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/lfsauthenticate"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/lfstransfer"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/personalaccesstoken"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/receivepack"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/disallowedcommand"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/twofactorrecover"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/twofactorverify"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/uploadarchive"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/uploadpack"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/metrics"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/sshenv"
)

// New creates a new command based on the provided arguments, environment, config, and readWriter
func New(
	arguments []string,
	env sshenv.Env,
	config *config.Config,
	readWriter *readwriter.ReadWriter,
	httpClient *client.HTTPClient,
) (command.Command, error) {
	args, err := Parse(arguments, env)
	if err != nil {
		return nil, err
	}

	if cmd := Build(args, config, readWriter, httpClient); cmd != nil {
		return cmd, nil
	}

	return nil, disallowedcommand.Error
}

// NewWithKey creates a new command with the provided key ID
func NewWithKey(gitlabKeyID string, env sshenv.Env, config *config.Config, readWriter *readwriter.ReadWriter) (command.Command, error) {
	args, err := Parse(nil, env)
	if err != nil {
		return nil, err
	}

	args.GitlabKeyID = gitlabKeyID
	//TODO
	if cmd := Build(args, config, readWriter, nil); cmd != nil {
		return cmd, nil
	}

	return nil, disallowedcommand.Error
}

// NewWithKrb5Principal creates a new command with the provided Kerberos 5 principal
func NewWithKrb5Principal(gitlabKrb5Principal string, env sshenv.Env, config *config.Config, readWriter *readwriter.ReadWriter) (command.Command, error) {
	args, err := Parse(nil, env)
	if err != nil {
		return nil, err
	}

	args.GitlabKrb5Principal = gitlabKrb5Principal
	// TODO
	if cmd := Build(args, config, readWriter, nil); cmd != nil {
		return cmd, nil
	}

	return nil, disallowedcommand.Error
}

// NewWithUsername creates a new command with the provided username
func NewWithUsername(gitlabUsername string, env sshenv.Env, config *config.Config, readWriter *readwriter.ReadWriter) (command.Command, error) {
	args, err := Parse(nil, env)
	if err != nil {
		return nil, err
	}

	if env.NamespacePath != "" {
		if !slices.Contains(commandargs.GitCommands, args.CommandType) {
			return nil, disallowedcommand.Error
		}
	}

	args.GitlabUsername = gitlabUsername
	// TODO
	if cmd := Build(args, config, readWriter, nil); cmd != nil {
		return cmd, nil
	}

	return nil, disallowedcommand.Error
}

// Parse parses the provided arguments and environment to create a commandargs.Shell object
func Parse(arguments []string, env sshenv.Env) (*commandargs.Shell, error) {
	args := &commandargs.Shell{Arguments: arguments, Env: env}

	if err := args.Parse(); err != nil {
		return nil, err
	}

	return args, nil
}

// Build constructs a command based on the provided arguments, config, and readWriter
func Build(args *commandargs.Shell, config *config.Config, readWriter *readwriter.ReadWriter, httpClient *client.HTTPClient) command.Command {
	gitlabClient, _ := client.NewGitlabNetClient(config.User, config.HTTPSettings.Password, config.Secret, httpClient)
	switch args.CommandType {
	case commandargs.Discover:
		return &discover.Command{GitlabClient: gitlabClient, Args: args, ReadWriter: readWriter}
	case commandargs.TwoFactorRecover:
		return &twofactorrecover.Command{GitlabClient: gitlabClient, Args: args, ReadWriter: readWriter}
	case commandargs.TwoFactorVerify:
		return &twofactorverify.Command{GitlabClient: gitlabClient, Args: args, ReadWriter: readWriter}
	case commandargs.LfsAuthenticate:
		metrics.LfsHTTPConnectionsTotal.Inc()
		return &lfsauthenticate.Command{GitlabClient: gitlabClient, Args: args, ReadWriter: readWriter}
	case commandargs.LfsTransfer:
		if config.LFSConfig.PureSSHProtocol {
			metrics.LfsSSHConnectionsTotal.Inc()
			return &lfstransfer.Command{Config: config, Args: args, ReadWriter: readWriter}
		}
	case commandargs.ReceivePack:
		return &receivepack.Command{Config: config, Args: args, ReadWriter: readWriter}
	case commandargs.UploadPack:
		return &uploadpack.Command{Config: config, Args: args, ReadWriter: readWriter}
	case commandargs.UploadArchive:
		return &uploadarchive.Command{Config: config, Args: args, ReadWriter: readWriter}
	case commandargs.PersonalAccessToken:
		if config.PATConfig.Enabled {
			return &personalaccesstoken.Command{Config: config, Args: args, ReadWriter: readWriter}
		}
	}

	return nil
}
