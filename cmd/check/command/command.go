package command

import (
	"gitlab.com/gitlab-org/gitlab-shell/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/healthcheck"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/shared/disallowedcommand"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
)


func New(config *config.Config, readWriter *readwriter.ReadWriter) (command.Command, error) {
	if cmd := build(config, readWriter); cmd != nil {
		return cmd, nil
	}

	return nil, disallowedcommand.Error
}

func build(config *config.Config, readWriter *readwriter.ReadWriter) command.Command {
	return &healthcheck.Command{Config: config, ReadWriter: readWriter}
}
