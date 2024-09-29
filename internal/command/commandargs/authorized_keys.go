// Package commandargs defines structures and methods for handling command-line arguments
// related to authorized key checks in the GitLab shell.
package commandargs

import (
	"errors"
	"fmt"
)

// AuthorizedKeys holds the arguments and user information for key authorization checks.
type AuthorizedKeys struct {
	Arguments    []string
	ExpectedUser string
	ActualUser   string
	Key          string
}

// Parse parses and validates the arguments, setting ExpectedUser, ActualUser, and Key.
func (ak *AuthorizedKeys) Parse() error {
	if err := ak.validate(); err != nil {
		return err
	}

	ak.ExpectedUser = ak.Arguments[0]
	ak.ActualUser = ak.Arguments[1]
	ak.Key = ak.Arguments[2]

	return nil
}

// GetArguments returns the list of command-line arguments.
func (ak *AuthorizedKeys) GetArguments() []string {
	return ak.Arguments
}

func (ak *AuthorizedKeys) validate() error {
	argsSize := len(ak.Arguments)

	if argsSize != 3 {
		return fmt.Errorf("# Insufficient arguments. %d. Usage\n#\tgitlab-shell-authorized-keys-check <expected-username> <actual-username> <key>", argsSize)
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
