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
	"gitlab.com/gitlab-org/labkit/tracing"
	"gitlab.com/gitlab-org/labkit/v2/featureflag"
	"gitlab.com/gitlab-org/labkit/v2/fields"
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

// LogDataKey is the context key used to store log data in request contexts.
const LogDataKey contextKey = "logData"

// featureFlagClientKey is the context key used to store the feature flag evaluator.
const featureFlagClientKey contextKey = "featureFlagClient"

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
//
// If the FEATURE_FLAG_ENDPOINT environment variable is set, a labkit v2
// feature flag client is created and stored in the returned context. Callers
// can retrieve it with FeatureFlagEvaluatorFromContext. If the endpoint is not
// configured the client is omitted and flag checks default to false — startup
// is never blocked by a missing Flipt server.
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

	ffClient, _ := featureflag.NewWithConfig(ctx, &featureflag.Config{
		Name:      serviceName,
		Endpoint:  config.FeatureFlags.Endpoint,
		Namespace: config.FeatureFlags.Namespace,
	})
	if ffClient != nil {
		ctx = context.WithValue(ctx, featureFlagClientKey, ffClient)
	}

	return ctx, func() {
		if ffClient != nil {
			if err := ffClient.Shutdown(ctx); err != nil {
				slog.WarnContext(ctx, "feature flag client shutdown error", slog.String("error", err.Error()))
			}
		}
		finished()
		_ = closer.Close()
	}
}

// FeatureFlagEvaluatorFromContext returns the feature flag evaluator stored in
// ctx by Setup, or nil if no evaluator was registered (e.g. FEATURE_FLAG_ENDPOINT
// is not set). Callers must treat a nil return as "all flags off".
func FeatureFlagEvaluatorFromContext(ctx context.Context) featureflag.Evaluator {
	v, _ := ctx.Value(featureFlagClientKey).(featureflag.Evaluator)
	return v
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
