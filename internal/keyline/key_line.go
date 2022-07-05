package keyline

import (
	"errors"
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
	PublicKeyPrefix = "key"
	PrincipalPrefix = "username"
	SshOptions      = "no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty"
)

type KeyLine struct {
	Id     string // This can be either an ID of a Key or username
	Value  string // This can be either a public key or a principal name
	Prefix string
	Config *config.Config
}

func NewPublicKeyLine(id, publicKey string, config *config.Config) (*KeyLine, error) {
	return newKeyLine(id, publicKey, PublicKeyPrefix, config)
}

func NewPrincipalKeyLine(keyId, principal string, config *config.Config) (*KeyLine, error) {
	return newKeyLine(keyId, principal, PrincipalPrefix, config)
}

func (k *KeyLine) ToString() string {
	command := fmt.Sprintf("%s %s-%s", path.Join(k.Config.RootDir, executable.BinDir, executable.GitlabShell), k.Prefix, k.Id)

	return fmt.Sprintf(`command="%s",%s %s`, command, SshOptions, k.Value)
}

func newKeyLine(id, value, prefix string, config *config.Config) (*KeyLine, error) {
	if err := validate(id, value); err != nil {
		return nil, err
	}

	return &KeyLine{Id: id, Value: value, Prefix: prefix, Config: config}, nil
}

func validate(id string, value string) error {
	if !keyRegex.MatchString(id) {
		return errors.New(fmt.Sprintf("Invalid key_id: %s", id))
	}

	if strings.Contains(value, "\n") {
		return errors.New(fmt.Sprintf("Invalid value: %s", value))
	}

	return nil
}
