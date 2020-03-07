package logger

import (
	"fmt"
	"io"
	"io/ioutil"
	golog "log"
	"log/syslog"
	"math"
	"os"
	"sync"
	"time"

	"gitlab.com/gitlab-org/gitlab-shell/internal/config"

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
	ProgName, _ = os.Executable()

	// Avoid leaking output if we can't set up the logging output
	log.SetOutput(ioutil.Discard)

	output, err := os.OpenFile(cfg.LogFile, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	if err != nil {
		setupBootstrapLogger()
		logPrint("Unable to configure logging", err)
		return err
	}

	logWriter = output
	log.SetOutput(logWriter)
	if cfg.LogFormat == "json" {
		log.SetFormatter(&log.JSONFormatter{})
	}

	return nil
}

// If our log file is not available we want to log somewhere else, but
// not to standard error because that leaks information to the user. This
// function attempts to log to syslog.
func logPrint(msg string, err error) {
	if logWriter == nil {
		if bootstrapLogger != nil {
			bootstrapLogger.Print(ProgName+":", msg+":", err)
		}
		return
	}

	log.WithError(err).WithFields(log.Fields{
		"pid": pid,
	}).Error(msg)
}

func Fatal(msg string, err error) {
	mutex.Lock()
	defer mutex.Unlock()
	setupBootstrapLogger()

	logPrint(msg, err)
	// We don't show the error to the end user because it can leak
	// information that is private to the GitLab server.
	fmt.Fprintf(os.Stderr, "%s: fatal: %s\n", ProgName, msg)
	os.Exit(1)
}

// We assume the logging mutex is already locked.
func setupBootstrapLogger() {
	if bootstrapLogger == nil {
		bootstrapLogger, _ = syslog.NewLogger(syslog.LOG_ERR|syslog.LOG_USER, 0)
	}
}

func ElapsedTimeMs(start time.Time, end time.Time) float64 {
	// Later versions of Go support Milliseconds directly:
	// https://go-review.googlesource.com/c/go/+/167387/
	return roundFloat(end.Sub(start).Seconds() * 1e3)
}

func roundFloat(x float64) float64 {
	return round(x, 1000)
}

func round(x, unit float64) float64 {
	return math.Round(x*unit) / unit
}
