package commandargs

import (
	"gitlab.com/gitlab-org/gitlab-shell/internal/executable"
)

type CommandType string

type CommandArgs interface {
	Parse() error
	GetArguments() []string
}

func Parse(e *executable.Executable, arguments []string) (CommandArgs, error) {
	var args CommandArgs = &GenericArgs{Arguments: arguments}

	switch e.Name {
	case executable.GitlabShell:
		args = &Shell{Arguments: arguments}
	case executable.AuthorizedKeysCheck:
		args = &AuthorizedKeys{Arguments: arguments}
	case executable.AuthorizedPrincipalsCheck:
		args = &AuthorizedPrincipals{Arguments: arguments}
	}

	if err := args.Parse(); err != nil {
		return nil, err
	}

	return args, nil
}
