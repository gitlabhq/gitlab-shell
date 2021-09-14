package commandargs

type CommandType string

type CommandArgs interface {
	Parse() error
	GetArguments() []string
}
