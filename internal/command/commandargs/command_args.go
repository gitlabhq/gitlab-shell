package commandargs

import (
	"errors"
	"fmt"

	"gitlab.com/gitlab-org/gitlab-shell/internal/executable"
	"gitlab.com/gitlab-org/gitlab-shell/internal/sshenv"
)

type CommandType string

type CommandArgs interface {
	Parse() error
	GetArguments() []string
}

func Parse(e *executable.Executable, arguments []string, env sshenv.Env) (CommandArgs, error) {
	var args CommandArgs

	switch e.Name {
	case executable.GitlabShell:
		args = &Shell{Arguments: arguments, Env: env}
	case executable.AuthorizedKeysCheck:
		args = &AuthorizedKeys{Arguments: arguments}
	case executable.AuthorizedPrincipalsCheck:
		args = &AuthorizedPrincipals{Arguments: arguments}
	default:
		return nil, errors.New(fmt.Sprintf("unknown executable: %s", e.Name))
	}

	if err := args.Parse(); err != nil {
		return nil, err
	}

	return args, nil
}
