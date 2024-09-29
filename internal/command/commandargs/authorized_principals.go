// Package commandargs provides functionality for handling and parsing command-line arguments
// related to authorized principals for GitLab shell commands.
package commandargs

import (
	"errors"
	"fmt"
)

// AuthorizedPrincipals holds the arguments for checking authorized principals and the key ID.
type AuthorizedPrincipals struct {
	Arguments  []string
	KeyID      string
	Principals []string
}

// Parse validates and extracts the key ID and principals from the Arguments slice.
// Returns an error if validation fails.
func (ap *AuthorizedPrincipals) Parse() error {
	if err := ap.validate(); err != nil {
		return err
	}

	ap.KeyID = ap.Arguments[0]
	ap.Principals = ap.Arguments[1:]

	return nil
}

// GetArguments returns the list of command-line arguments provided.
func (ap *AuthorizedPrincipals) GetArguments() []string {
	return ap.Arguments
}

func (ap *AuthorizedPrincipals) validate() error {
	argsSize := len(ap.Arguments)

	if argsSize < 2 {
		return fmt.Errorf("# Insufficient arguments. %d. Usage\n#\tgitlab-shell-authorized-principals-check <key-id> <principal1> [<principal2>...]", argsSize)
	}

	keyID := ap.Arguments[0]
	principals := ap.Arguments[1:]

	if keyID == "" {
		return errors.New("# No key_id provided")
	}

	for _, principal := range principals {
		if principal == "" {
			return errors.New("# An invalid principal was provided")
		}
	}

	return nil
}
