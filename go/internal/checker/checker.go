package checker

import (
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/checker/authorizedkeys"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/checker/authorizedprincipals"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/executable"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/executable/fallback"
)

const (
	AuthorizedKeys       = "gitlab-shell-authorized-keys-check"
	AuthorizedPrincipals = "gitlab-shell-authorized-principals-check"
)

type Checker interface {
	Execute() error
}

func New(e *executable.Executable, arguments []string, config *config.Config, readWriter *readwriter.ReadWriter) Checker {
	if config.FeatureEnabled(e.Name) {
		if checker := buildChecker(e.Name, arguments[1:], config, readWriter); checker != nil {
			return checker
		}
	}

	return &fallback.Executable{Program: e.FallbackProgram(), RootDir: config.RootDir, Args: arguments}
}

func buildChecker(name string, args []string, config *config.Config, readWriter *readwriter.ReadWriter) Checker {
	switch name {
	case AuthorizedKeys:
		return &authorizedkeys.Checker{Config: config, Args: args, ReadWriter: readWriter}
	case AuthorizedPrincipals:
		return &authorizedprincipals.Checker{Config: config, Args: args, ReadWriter: readWriter}
	}

	return nil
}
