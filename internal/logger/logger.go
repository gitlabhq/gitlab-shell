// Package logger provides logging configuration utilities for the gitlab-shell
package logger

import (
	"io"
	"log/slog"
	"os"

	v2log "gitlab.com/gitlab-org/labkit/v2/log"
)

// Log - represents a singleton logger.
var Log *slog.Logger

// LogOptions - allows you to configure the logger being used within
// gitlab-shell.
type LogOptions struct {
	LogLevel string
	LogFile  string
}

func init() {
	Log = v2log.New()
}

// ConfigureLogger - gitlab-sshd's log output can be configured to text as per the documentation:
// https://docs.gitlab.com/omnibus/settings/logs/#json-logging
// This is currently controlled by the GITLAB_LOG_FORMAT environment variable.
func ConfigureLogger(cfg *LogOptions) (*slog.Logger, io.Closer, error) {
	logConfig := &v2log.Config{
		LogLevel: parseLogLevel(cfg.LogLevel),
	}
	if gitlabLogFormat := os.Getenv("GITLAB_LOG_FORMAT"); gitlabLogFormat == "text" {
		logConfig.UseTextFormat = true
	}
	var err error
	var closer io.Closer
	Log, closer, err = v2log.NewWithFile(cfg.LogFile, logConfig)
	if err != nil {
		return Log, nil, err
	}

	return Log, closer, err
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
