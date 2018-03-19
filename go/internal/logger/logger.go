package logger

import (
	"fmt"
	"io"
	golog "log"
	"log/syslog"
	"os"
	"sync"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/config"

	log "github.com/sirupsen/logrus"
)

var (
	logWriter       io.Writer
	bootstrapLogger *golog.Logger
	pid             int
	mutex           sync.Mutex
	ProgName        string
)

func Configure(cfg *config.Config) error {
	mutex.Lock()
	defer mutex.Unlock()

	pid = os.Getpid()

	var err error
	logWriter, err = os.OpenFile(cfg.LogFile, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return err
	}

	log.SetOutput(logWriter)
	if cfg.LogFormat == "json" {
		log.SetFormatter(&log.JSONFormatter{})
	}

	return nil
}

func logPrint(msg string, err error) {
	mutex.Lock()
	defer mutex.Unlock()

	if logWriter == nil {
		bootstrapLogPrint(msg, err)
		return
	}

	log.WithError(err).WithFields(log.Fields{
		"pid": pid,
	}).Error(msg)
}

func Fatal(msg string, err error) {
	logPrint(msg, err)
	// We don't show the error to the end user because it can leak
	// information that is private to the GitLab server.
	fmt.Fprintf(os.Stderr, "%s: fatal: %s\n", ProgName, msg)
	os.Exit(1)
}

// If our log file is not available we want to log somewhere else, but
// not to standard error because that leaks information to the user. This
// function attemps to log to syslog.
//
// We assume the logging mutex is already locked.
func bootstrapLogPrint(msg string, err error) {
	if bootstrapLogger == nil {
		var err error
		bootstrapLogger, err = syslog.NewLogger(syslog.LOG_ERR|syslog.LOG_USER, 0)
		if err != nil {
			// The message will not be logged.
			return
		}
	}

	bootstrapLogger.Print(ProgName+":", msg+":", err)
}
