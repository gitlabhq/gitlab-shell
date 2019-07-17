package keyline

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
)

type KeyLine struct {
	Id      string
	Value   string
	Prefix  string
	RootDir string
}

const (
	GitlabShellDir     = "bin"
	GitlabShellProgram = "gitlab-shell"
)

func (k *KeyLine) ToString() (string, error) {
	if err := k.validate(); err != nil {
		return "", err
	}

	command := fmt.Sprintf("%s %s-%s", path.Join(k.RootDir, GitlabShellDir, GitlabShellProgram), k.Prefix, k.Id)

	return fmt.Sprintf(`command="%s",no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty %s`, command, k.Value), nil
}

func (k *KeyLine) validate() error {
	isMatched, err := regexp.MatchString(`\A[a-z0-9-]+\z`, k.Id)
	if err != nil {
		return err
	}

	if !isMatched {
		return errors.New(fmt.Sprintf("Invalid key_id: %s", k.Id))
	}

	if strings.Contains(k.Value, "\n") {
		return errors.New(fmt.Sprintf("Invalid value: %s", k.Value))
	}

	return nil
}
