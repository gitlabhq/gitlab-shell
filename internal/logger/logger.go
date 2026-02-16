// Package logger provides logging configuration utilities for the gitlab-shell
package logger

import (
	"fmt"
	"io"
	"log/slog"
	"log/syslog"
	"os"
	"time"

	"gitlab.com/gitlab-org/labkit/log"
	v2log "gitlab.com/gitlab-org/labkit/v2/log"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
)

// ConfigureLogger - gitlab-sshd's log output can be configured to text as per the documentation:
// https://docs.gitlab.com/omnibus/settings/logs/#json-logging
// This is currently controlled by the GITLAB_LOG_FORMAT environment variable.
func ConfigureLogger(cfg *config.Config) io.Closer {
	logConfig := &v2log.Config{
		LogLevel: parseLogLevel(cfg.LogLevel),
	}
	if gitlabLogFormat := os.Getenv("GITLAB_LOG_FORMAT"); gitlabLogFormat == "text" {
		logConfig.UseTextFormat = true
	}
	logger, logCloser, err := v2log.NewWithFile(cfg.LogFile, logConfig)
	slog.SetDefault(logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to configure log file %q: %v\n", cfg.LogFile, err)
		return nil
	}

	return logCloser
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

func syslogError(err error) {
	syslogLogger, syslogLoggerErr := syslog.NewLogger(syslog.LOG_ERR|syslog.LOG_USER, 0)
	progName, _ := os.Executable()
	if syslogLoggerErr == nil {
		msg := fmt.Sprintf("%s: Unable to configure logging: %v\n", progName, err.Error())
		syslogLogger.Print(msg)
	} else {
		msg := fmt.Sprintf("%s: Unable to configure logging: %v, %v\n", progName, err.Error(), syslogLoggerErr.Error())
		fmt.Fprintln(os.Stderr, msg)
	}
}

func logFmt(inFmt string) string {
	// Hide the "combined" format, since that makes no sense in gitlab-shell.
	// The default is JSON when unspecified.
	if inFmt == "" || inFmt == "combined" {
		return "json"
	}

	return inFmt
}

func logLevel(inLevel string) string {
	if inLevel == "" {
		return "info"
	}

	return inLevel
}

func logFile(inFile string) string {
	if inFile == "" {
		return "stderr"
	}

	return inFile
}

func buildOpts(cfg *config.Config) []log.LoggerOption {
	return []log.LoggerOption{
		log.WithFormatter(logFmt(cfg.LogFormat)),
		log.WithOutputName(logFile(cfg.LogFile)),
		log.WithTimezone(time.UTC),
		log.WithLogLevel(logLevel(cfg.LogLevel)),
	}
}

// Configure configures the logging singleton for operation inside a remote TTY (like SSH). In this
// mode an empty LogFile is not accepted and syslog is used as a fallback when LogFile could not be
// opened for writing.
func Configure(cfg *config.Config) io.Closer {
	var closer io.Closer = io.NopCloser(nil)
	err := fmt.Errorf("no logfile specified")

	if cfg.LogFile != "" {
		closer, err = log.Initialize(buildOpts(cfg)...)
	}

	if err != nil {
		syslogError(err)

		cfg.LogFile = "/dev/null"
		closer, err = log.Initialize(buildOpts(cfg)...)
		if err != nil {
			log.WithError(err).Warn("Unable to configure logging to /dev/null, leaving unconfigured")
		}
	}

	return closer
}
