// Package command provides the core command execution infrastructure for gitlab-shell.
// It defines the Command interface that all shell commands must implement,
// along with shared utilities for logging, tracing, and context management.
package command

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path"
	"strings"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/labkit/correlation"
	"gitlab.com/gitlab-org/labkit/fields"
	"gitlab.com/gitlab-org/labkit/tracing"
	"gitlab.com/gitlab-org/labkit/v2/log"
)

// Command is the interface that all gitlab-shell commands must implement.
// Execute runs the command and returns the updated context and any error.
type Command interface {
	Execute(ctx context.Context) (context.Context, error)
}

// LogMetadata contains project and namespace information for structured logging.
type LogMetadata struct {
	Project         string `json:"project,omitempty"`
	RootNamespace   string `json:"root_namespace,omitempty"`
	ProjectID       int    `json:"project_id,omitempty"`
	RootNamespaceID int    `json:"root_namespace_id,omitempty"`
}

// LogData contains user and request information for structured logging.
type LogData struct {
	Username     string      `json:"username"`
	WrittenBytes int64       `json:"written_bytes"`
	Meta         LogMetadata `json:"meta"`
}

type contextKey string

// LogDataKey is the context key used to store ldog data in request contexts.
const LogDataKey contextKey = "logData"

// CheckForVersionFlag checks if the -version flag was passed and prints version info if so.
// It exits the program after printing the version.
func CheckForVersionFlag(osArgs []string, version, buildTime string) {
	// We can't use the flag library because gitlab-shell receives other arguments
	// that confuse the parser.
	//
	// See: https://gitlab.com/gitlab-org/gitlab-shell/-/merge_requests/800#note_1459474735
	if len(osArgs) == 2 && osArgs[1] == "-version" {
		fmt.Printf("%s %s-%s\n", path.Base(osArgs[0]), version, buildTime)
		os.Exit(0)
	}
}

// Setup initializes tracing from the configuration file and generates a
// background context from which all other contexts in the process should derive
// from, as it has a service name and initial correlation ID set.
func Setup(serviceName string, config *config.Config) (context.Context, func()) {
	closer := tracing.Initialize(
		tracing.WithServiceName(serviceName),

		// For GitLab-Shell, we explicitly initialize tracing from a config file
		// instead of the default environment variable (using GITLAB_TRACING)
		// This decision was made owing to the difficulty in passing environment
		// variables into GitLab-Shell processes.
		//
		// Processes are spawned as children of the SSH daemon, which tightly
		// controls environment variables; doing this means we don't have to
		// enable PermitUserEnvironment
		//
		// gitlab-sshd could use the standard GITLAB_TRACING envvar, but that
		// would lead to inconsistencies between the two forms of operation
		tracing.WithConnectionString(config.GitlabTracing),
	)

	ctx, finished := tracing.ExtractFromEnv(context.Background())
	ctx = correlation.ContextWithClientName(ctx, serviceName)

	correlationID := correlation.ExtractFromContext(ctx)
	if correlationID == "" {
		correlationID := correlation.SafeRandomID()
		ctx = correlation.ContextWithCorrelation(ctx, correlationID)
		ctx = log.WithFields(ctx, slog.String(fields.CorrelationID, correlationID))
	}

	return ctx, func() {
		finished()
		_ = closer.Close()
	}
}

// NewLogData creates a new LogData instance with the given project, username, and IDs.
// It extracts the root namespace from the project path.
func NewLogData(project, username string, projectID, rootNamespaceID int) LogData {
	rootNameSpace := ""

	if len(project) > 0 {
		splitFn := func(c rune) bool {
			return c == '/'
		}
		m := strings.FieldsFunc(project, splitFn)
		if len(m) > 0 {
			rootNameSpace = m[0]
		}
	}

	return LogData{
		Username:     username,
		WrittenBytes: 0,
		Meta: LogMetadata{
			Project:         project,
			RootNamespace:   rootNameSpace,
			ProjectID:       projectID,
			RootNamespaceID: rootNamespaceID,
		},
	}
}
