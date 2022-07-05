package logger

import (
	"fmt"
	"io"
	"log/syslog"
	"os"
	"time"

	"gitlab.com/gitlab-org/labkit/log"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
)

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
	err := fmt.Errorf("No logfile specified")

	if cfg.LogFile != "" {
		closer, err = log.Initialize(buildOpts(cfg)...)
	}

	if err != nil {
		progName, _ := os.Executable()
		syslogLogger, syslogLoggerErr := syslog.NewLogger(syslog.LOG_ERR|syslog.LOG_USER, 0)
		if syslogLoggerErr == nil {
			msg := fmt.Sprintf("%s: Unable to configure logging: %v\n", progName, err.Error())
			syslogLogger.Print(msg)
		} else {
			msg := fmt.Sprintf("%s: Unable to configure logging: %v, %v\n", progName, err.Error(), syslogLoggerErr.Error())
			fmt.Fprintf(os.Stderr, msg)
		}

		cfg.LogFile = "/dev/null"
		closer, err = log.Initialize(buildOpts(cfg)...)
		if err != nil {
			log.WithError(err).Warn("Unable to configure logging to /dev/null, leaving unconfigured")
		}
	}

	return closer
}

// ConfigureStandalone configures the logging singleton for standalone operation. In this mode an
// empty LogFile is treated as logging to stderr, and standard output is used as a fallback
// when LogFile could not be opened for writing.
func ConfigureStandalone(cfg *config.Config) io.Closer {
	closer, err1 := log.Initialize(buildOpts(cfg)...)
	if err1 != nil {
		var err2 error

		cfg.LogFile = "stdout"
		closer, err2 = log.Initialize(buildOpts(cfg)...)

		// Output this after the logger has been configured!
		log.WithError(err1).WithField("log_file", cfg.LogFile).Warn("Unable to configure logging, falling back to STDOUT")

		// LabKit v1.7.0 doesn't have any conditions where logging to "stdout" will fail
		if err2 != nil {
			log.WithError(err2).Warn("Unable to configure logging to STDOUT, leaving unconfigured")
		}
	}

	return closer
}
