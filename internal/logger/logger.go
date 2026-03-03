// Package logger provides logging configuration utilities for the gitlab-shell
package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"

	v2log "gitlab.com/gitlab-org/labkit/v2/log"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
)

// ConfigureLogger - gitlab-sshd's log output can be configured to text as per the documentation:
// https://docs.gitlab.com/omnibus/settings/logs/#json-logging
// This is currently controlled by the GITLAB_LOG_FORMAT environment variable.
func ConfigureLogger(cfg *config.Config) io.Closer {
	logConfig := &v2log.Config{}
	if gitlabLogFormat := os.Getenv("GITLAB_LOG_FORMAT"); gitlabLogFormat == "text" {
		logConfig.UseTextFormat = true
	}
	if cfg.LogLevel != "" {
		var level slog.Level
		if err := level.UnmarshalText([]byte(cfg.LogLevel)); err == nil {
			logConfig.LogLevel = &level
		}
	}
	logger, logCloser, err := v2log.NewWithFile(cfg.LogFile, logConfig)
	slog.SetDefault(logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to configure log file %q: %v\n", cfg.LogFile, err)
		return nil
	}

	return logCloser
}
