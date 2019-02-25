package handler

import (
	"context"
	"fmt"
	"os"

	"gitlab.com/gitlab-org/gitaly/auth"
	"gitlab.com/gitlab-org/gitaly/client"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/logger"
	"gitlab.com/gitlab-org/labkit/tracing"
	"google.golang.org/grpc"
)

// GitalyHandlerFunc implementations are responsible for deserializing
// the request JSON into a GRPC request message, making an appropriate Gitaly
// call with the request, using the provided client, and returning the exit code
// or error from the Gitaly call.
type GitalyHandlerFunc func(ctx context.Context, client *grpc.ClientConn, requestJSON string) (int32, error)

// RunGitalyCommand provides a bootstrap for Gitaly commands executed
// through GitLab-Shell. It ensures that logging, tracing and other
// common concerns are configured before executing the `handler`.
// RunGitalyCommand will handle errors internally and call
// `os.Exit()` on completion. This method will never return to
// the caller.
func RunGitalyCommand(handler GitalyHandlerFunc) {
	exitCode, err := internalRunGitalyCommand(os.Args, handler)

	if err != nil {
		logger.Fatal("error: %v", err)
	}

	os.Exit(exitCode)
}

// internalRunGitalyCommand is like RunGitalyCommand, except that since it doesn't
// call os.Exit, we can rely on its deferred handlers executing correctly
func internalRunGitalyCommand(args []string, handler GitalyHandlerFunc) (int, error) {

	if len(args) != 3 {
		return 1, fmt.Errorf("expected 2 arguments, got %v", args)
	}

	cfg, err := config.New()
	if err != nil {
		return 1, err
	}

	if err := logger.Configure(cfg); err != nil {
		return 1, err
	}

	// Use a working directory that won't get removed or unmounted.
	if err := os.Chdir("/"); err != nil {
		return 1, err
	}

	// Configure distributed tracing
	serviceName := fmt.Sprintf("gitlab-shell-%v", args[0])
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
		tracing.WithConnectionString(cfg.GitlabTracing),
	)
	defer closer.Close()

	ctx, finished := tracing.ExtractFromEnv(context.Background())
	defer finished()

	gitalyAddress := args[1]
	if gitalyAddress == "" {
		return 1, fmt.Errorf("no gitaly_address given")
	}

	conn, err := client.Dial(gitalyAddress, dialOpts())
	if err != nil {
		return 1, err
	}
	defer conn.Close()

	requestJSON := string(args[2])
	exitCode, err := handler(ctx, conn, requestJSON)
	return int(exitCode), err
}

func dialOpts() []grpc.DialOption {
	connOpts := client.DefaultDialOpts
	if token := os.Getenv("GITALY_TOKEN"); token != "" {
		connOpts = append(client.DefaultDialOpts, grpc.WithPerRPCCredentials(gitalyauth.RPCCredentialsV2(token)))
	}

	return connOpts
}
