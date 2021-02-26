package logger

import (
	"fmt"
	"io/ioutil"
	"log/syslog"
	"os"

	log "github.com/sirupsen/logrus"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
)

func configureLogFormat(cfg *config.Config) {
	if cfg.LogFormat == "json" {
		log.SetFormatter(&log.JSONFormatter{})
	}
}

// Configure configures the logging singleton for operation inside a remote TTY (like SSH). In this
// mode an empty LogFile is not accepted and syslog is used as a fallback when LogFile could not be
// opened for writing.
func Configure(cfg *config.Config) {
	logFile, err := os.OpenFile(cfg.LogFile, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
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

		// Discard logs since a log file was specified but couldn't be opened
		log.SetOutput(ioutil.Discard)
	}

	log.SetOutput(logFile)

	configureLogFormat(cfg)
}

// ConfigureStandalone configures the logging singleton for standalone operation. In this mode an
// empty LogFile is treated as logging to standard output and standard output is used as a fallback
// when LogFile could not be opened for writing.
func ConfigureStandalone(cfg *config.Config) {
	if cfg.LogFile == "" {
		return
	}

	logFile, err := os.OpenFile(cfg.LogFile, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	if err != nil {
		log.Printf("Unable to configure logging, falling back to stdout: %v", err)
		return
	}
	log.SetOutput(logFile)

	configureLogFormat(cfg)
}
