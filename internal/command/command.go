package command

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/labkit/correlation"
	"gitlab.com/gitlab-org/labkit/tracing"
)

type Command interface {
	Execute(ctx context.Context) (context.Context, error)
}

type LogMetadata struct {
	Project         string `json:"project,omitempty"`
	RootNamespace   string `json:"root_namespace,omitempty"`
	ProjectID       int    `json:"project_id,omitempty"`
	RootNamespaceID int    `json:"root_namespace_id,omitempty"`
}

type LogData struct {
	Username     string      `json:"username"`
	WrittenBytes int64       `json:"written_bytes"`
	Meta         LogMetadata `json:"meta"`
}

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

// Setup() initializes tracing from the configuration file and generates a
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
	}

	return ctx, func() {
		finished()
		closer.Close()
	}
}

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
