package commandargs

import (
	"errors"
	"fmt"
)

type AuthorizedPrincipals struct {
	Arguments  []string
	KeyId      string
	Principals []string
}

func (ap *AuthorizedPrincipals) Parse() error {
	if err := ap.validate(); err != nil {
		return err
	}

	ap.KeyId = ap.Arguments[0]
	ap.Principals = ap.Arguments[1:]

	return nil
}

func (ap *AuthorizedPrincipals) GetArguments() []string {
	return ap.Arguments
}

func (ap *AuthorizedPrincipals) validate() error {
	argsSize := len(ap.Arguments)

	if argsSize < 2 {
		return errors.New(fmt.Sprintf("# Insufficient arguments. %d. Usage\n#\tgitlab-shell-authorized-principals-check <key-id> <principal1> [<principal2>...]", argsSize))
	}

	keyId := ap.Arguments[0]
	principals := ap.Arguments[1:]

	if keyId == "" {
		return errors.New("# No key_id provided")
	}

	for _, principal := range principals {
		if principal == "" {
			return errors.New("# An invalid principal was provided")
		}
	}

	return nil
}
