package commandargs

import (
	"errors"
	"fmt"
)

type AuthorizedKeys struct {
	Arguments    []string
	ExpectedUser string
	ActualUser   string
	Key          string
}

func (ak *AuthorizedKeys) Parse() error {
	if err := ak.validate(); err != nil {
		return err
	}

	ak.ExpectedUser = ak.Arguments[0]
	ak.ActualUser = ak.Arguments[1]
	ak.Key = ak.Arguments[2]

	return nil
}

func (ak *AuthorizedKeys) GetArguments() []string {
	return ak.Arguments
}

func (ak *AuthorizedKeys) validate() error {
	argsSize := len(ak.Arguments)

	if argsSize != 3 {
		return errors.New(fmt.Sprintf("# Insufficient arguments. %d. Usage\n#\tgitlab-shell-authorized-keys-check <expected-username> <actual-username> <key>", argsSize))
	}

	expectedUsername := ak.Arguments[0]
	actualUsername := ak.Arguments[1]
	key := ak.Arguments[2]

	if expectedUsername == "" || actualUsername == "" {
		return errors.New("# No username provided")
	}

	if key == "" {
		return errors.New("# No key provided")
	}

	return nil
}
