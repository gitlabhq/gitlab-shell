package commandargs

type CommandType string
type Executable string

const (
	GitlabShell Executable = "gitlab-shell"
)

type CommandArgs interface {
	Parse() error
	Executable() Executable
	Arguments() []string
}

func Parse(arguments []string) (CommandArgs, error) {
	var args CommandArgs = &BaseArgs{arguments: arguments}

	switch args.Executable() {
	case GitlabShell:
		args = &Shell{BaseArgs: args.(*BaseArgs)}
	}

	if err := args.Parse(); err != nil {
		return nil, err
	}

	return args, nil
}
