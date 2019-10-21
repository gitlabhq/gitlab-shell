package keyline

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"

	"gitlab.com/gitlab-org/gitlab-shell/internal/executable"
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
	Id      string // This can be either an ID of a Key or username
	Value   string // This can be either a public key or a principal name
	Prefix  string
	RootDir string
}

func NewPublicKeyLine(id string, publicKey string, rootDir string) (*KeyLine, error) {
	return newKeyLine(id, publicKey, PublicKeyPrefix, rootDir)
}

func NewPrincipalKeyLine(keyId string, principal string, rootDir string) (*KeyLine, error) {
	return newKeyLine(keyId, principal, PrincipalPrefix, rootDir)
}

func (k *KeyLine) ToString() string {
	command := fmt.Sprintf("%s %s-%s", path.Join(k.RootDir, executable.BinDir, executable.GitlabShell), k.Prefix, k.Id)

	return fmt.Sprintf(`command="%s",%s %s`, command, SshOptions, k.Value)
}

func newKeyLine(id string, value string, prefix string, rootDir string) (*KeyLine, error) {
	if err := validate(id, value); err != nil {
		return nil, err
	}

	return &KeyLine{Id: id, Value: value, Prefix: prefix, RootDir: rootDir}, nil
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
