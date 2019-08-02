package commandargs

type GenericArgs struct {
	Arguments []string
}

func (b *GenericArgs) Parse() error {
	// Do nothing
	return nil
}

func (b *GenericArgs) GetArguments() []string {
	return b.Arguments
}
