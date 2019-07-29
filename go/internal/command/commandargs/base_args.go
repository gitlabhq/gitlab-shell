package commandargs

import (
	"errors"
	"path/filepath"
)

type BaseArgs struct {
	arguments []string
}

func (b *BaseArgs) Parse() error {
	if b.hasEmptyArguments() {
		return errors.New("arguments should include the executable")
	}

	return nil
}

func (b *BaseArgs) Executable() Executable {
	if b.hasEmptyArguments() {
		return Executable("")
	}

	return Executable(filepath.Base(b.arguments[0]))
}

func (b *BaseArgs) Arguments() []string {
	return b.arguments[1:]
}

func (b *BaseArgs) hasEmptyArguments() bool {
	return len(b.arguments) == 0
}
