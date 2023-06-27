package command

import (
	"strings"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/discover"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/lfsauthenticate"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/personalaccesstoken"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/receivepack"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/disallowedcommand"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/twofactorrecover"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/twofactorverify"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/uploadarchive"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/uploadpack"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/sshenv"
)

type MetaData struct {
	Username      string `json:"username"`
	Project       string `json:"project,omitempty"`
	RootNamespace string `json:"root_namespace,omitempty"`
}

func NewMetaData(project, username string) MetaData {
	rootNameSpace := ""

	if len(project) > 0 {
		splitFn := func(c rune) bool {
			return c == '/'
		}
		m := strings.FieldsFunc(project, splitFn)
		if len(m) > 0 {
			rootNameSpace = m[0]
		}
	}

	return MetaData{
		Username:      username,
		Project:       project,
		RootNamespace: rootNameSpace,
	}
}

func New(arguments []string, env sshenv.Env, config *config.Config, readWriter *readwriter.ReadWriter) (command.Command, error) {
	args, err := Parse(arguments, env)
	if err != nil {
		return nil, err
	}

	if cmd := Build(args, config, readWriter); cmd != nil {
		return cmd, nil
	}

	return nil, disallowedcommand.Error
}

func NewWithKey(gitlabKeyId string, env sshenv.Env, config *config.Config, readWriter *readwriter.ReadWriter) (command.Command, error) {
	args, err := Parse(nil, env)
	if err != nil {
		return nil, err
	}

	args.GitlabKeyId = gitlabKeyId
	if cmd := Build(args, config, readWriter); cmd != nil {
		return cmd, nil
	}

	return nil, disallowedcommand.Error
}

func NewWithKrb5Principal(gitlabKrb5Principal string, env sshenv.Env, config *config.Config, readWriter *readwriter.ReadWriter) (command.Command, error) {
	args, err := Parse(nil, env)
	if err != nil {
		return nil, err
	}

	args.GitlabKrb5Principal = gitlabKrb5Principal
	if cmd := Build(args, config, readWriter); cmd != nil {
		return cmd, nil
	}

	return nil, disallowedcommand.Error
}

func Parse(arguments []string, env sshenv.Env) (*commandargs.Shell, error) {
	args := &commandargs.Shell{Arguments: arguments, Env: env}

	if err := args.Parse(); err != nil {
		return nil, err
	}

	return args, nil
}

func Build(args *commandargs.Shell, config *config.Config, readWriter *readwriter.ReadWriter) command.Command {
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
