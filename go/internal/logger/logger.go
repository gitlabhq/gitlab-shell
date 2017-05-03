package logger

import (
	"fmt"
	"io"
	"log"
	"log/syslog"
	"os"
	"sync"
	"time"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/config"
)

var (
	logWriter       io.Writer
	bootstrapLogger *log.Logger
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
	return err
}

func logPrint(msg string, err error) {
	mutex.Lock()
	defer mutex.Unlock()

	if logWriter == nil {
		bootstrapLogPrint(msg, err)
		return
	}

	// Emulate the existing log format of gitlab-shell
	t := time.Now().Format("2006-01-02T15:04:05.999999")
	prefix := fmt.Sprintf("E, [%s #%d] ERROR -- : %s:", t, pid, ProgName)
	fmt.Fprintf(logWriter, "%s %s: %v\n", prefix, msg, err)
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
