// Package logger provides logging configuration utilities for the gitlab-shell
package logger

import (
	"io"
	"log/slog"
	"os"

	v2log "gitlab.com/gitlab-org/labkit/v2/log"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
)

// ConfigureLogger - gitlab-sshd's log output can be configured to text as per the documentation:
// https://docs.gitlab.com/omnibus/settings/logs/#json-logging
// This is currently controlled by the GITLAB_LOG_FORMAT environment variable.
func ConfigureLogger(cfg *config.Config) (*slog.Logger, io.Closer, error) {
	logConfig := &v2log.Config{
		LogLevel: parseLogLevel(cfg.LogLevel),
	}
	if gitlabLogFormat := os.Getenv("GITLAB_LOG_FORMAT"); gitlabLogFormat == "text" {
		logConfig.UseTextFormat = true
	}
	return v2log.NewWithFile(cfg.LogFile, logConfig)
}

func parseLogLevel(level string) slog.Level {
	switch level {
	case "DEBUG", "debug":
		return slog.LevelDebug
	case "INFO", "info":
		return slog.LevelInfo
	case "WARN", "warn":
		return slog.LevelWarn
	case "ERROR", "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
