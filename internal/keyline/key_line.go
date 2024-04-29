// Package keyline provides functionality for managing SSH key lines
package keyline

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/executable"
)

var (
	keyRegex = regexp.MustCompile(`\A[a-z0-9-]+\z`)
)

const (
	// PublicKeyPrefix is the prefix used for public keys
	PublicKeyPrefix = "key"

	// PrincipalPrefix is the prefix used for principals
	PrincipalPrefix = "username"

	// SSHOptions specifies SSH options for key lines
	SSHOptions = "no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty"
)

// KeyLine represents a struct used for SSH key management
type KeyLine struct {
	ID     string // This can be either an ID of a Key or username
	Value  string // This can be either a public key or a principal name
	Prefix string
	Config *config.Config
}

// NewPublicKeyLine creates a new KeyLine for a public key
func NewPublicKeyLine(id, publicKey string, config *config.Config) (*KeyLine, error) {
	return newKeyLine(id, publicKey, PublicKeyPrefix, config)
}

// NewPrincipalKeyLine creates a new KeyLine for a principal
func NewPrincipalKeyLine(keyID, principal string, config *config.Config) (*KeyLine, error) {
	return newKeyLine(keyID, principal, PrincipalPrefix, config)
}

// ToString converts a KeyLine to a string representation
func (k *KeyLine) ToString() string {
	command := fmt.Sprintf("%s %s-%s", path.Join(k.Config.RootDir, executable.BinDir, executable.GitlabShell), k.Prefix, k.ID)

	return fmt.Sprintf(`command="%s",%s %s`, command, SSHOptions, k.Value)
}

func newKeyLine(id, value, prefix string, config *config.Config) (*KeyLine, error) {
	if err := validate(id, value); err != nil {
		return nil, err
	}

	return &KeyLine{ID: id, Value: value, Prefix: prefix, Config: config}, nil
}

func validate(id string, value string) error {
	if !keyRegex.MatchString(id) {
		return fmt.Errorf("invalid key_id: %s", id)
	}

	if strings.Contains(value, "\n") {
		return fmt.Errorf("invalid value: %s", value)
	}

	return nil
}
