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

// GitalyHandlerFuncWithJSON implementations are responsible for deserializing
// the request JSON into a GRPC request message, making an appropriate Gitaly
// call with the request, using the provided client, and returning the exit code
// or error from the Gitaly call.
type GitalyHandlerFuncWithJSON func(ctx context.Context, client *grpc.ClientConn, requestJSON string) (int32, error)

// GitalyHandlerFunc implementations are responsible for making
// an appropriate Gitaly call using the provided client and context
// and returning an error from the Gitaly call.
type GitalyHandlerFunc func(ctx context.Context, client *grpc.ClientConn) (int32, error)

type GitalyConn struct {
	ctx   context.Context
	conn  *grpc.ClientConn
	close func()
}

type GitalyCommand struct {
	Config      *config.Config
	ServiceName string
	Address     string
	Token       string
}

// RunGitalyCommand provides a bootstrap for Gitaly commands executed
// through GitLab-Shell. It ensures that logging, tracing and other
// common concerns are configured before executing the `handler`.
// RunGitalyCommand will handle errors internally and call
// `os.Exit()` on completion. This method will never return to
// the caller.
func RunGitalyCommand(handler GitalyHandlerFuncWithJSON) {
	exitCode, err := internalRunGitalyCommand(os.Args, handler)

	if err != nil {
		logger.Fatal("error: %v", err)
	}

	os.Exit(exitCode)
}

// RunGitalyCommand provides a bootstrap for Gitaly commands executed
// through GitLab-Shell. It ensures that logging, tracing and other
// common concerns are configured before executing the `handler`.
func (gc *GitalyCommand) RunGitalyCommand(handler GitalyHandlerFunc) error {
	gitalyConn, err := getConn(gc)

	if err != nil {
		return err
	}

	_, err = handler(gitalyConn.ctx, gitalyConn.conn)

	gitalyConn.close()

	return err
}

// internalRunGitalyCommand runs Gitaly's command by particular Gitaly address and token
func internalRunGitalyCommand(args []string, handler GitalyHandlerFuncWithJSON) (int, error) {
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

	gc := &GitalyCommand{
		Config:      cfg,
		ServiceName: args[0],
		Address:     args[1],
		Token:       os.Getenv("GITALY_TOKEN"),
	}
	requestJSON := string(args[2])

	gitalyConn, err := getConn(gc)

	if err != nil {
		return 1, err
	}

	exitCode, err := handler(gitalyConn.ctx, gitalyConn.conn, requestJSON)

	gitalyConn.close()

	return int(exitCode), err
}

func getConn(gc *GitalyCommand) (*GitalyConn, error) {
	if gc.Address == "" {
		return nil, fmt.Errorf("no gitaly_address given")
	}

	connOpts := client.DefaultDialOpts
	if gc.Token != "" {
		connOpts = append(client.DefaultDialOpts, grpc.WithPerRPCCredentials(gitalyauth.RPCCredentialsV2(gc.Token)))
	}

	// Use a working directory that won't get removed or unmounted.
	if err := os.Chdir("/"); err != nil {
		return nil, err
	}

	// Configure distributed tracing
	serviceName := fmt.Sprintf("gitlab-shell-%v", gc.ServiceName)
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
		tracing.WithConnectionString(gc.Config.GitlabTracing),
	)

	ctx, finished := tracing.ExtractFromEnv(context.Background())

	conn, err := client.Dial(gc.Address, connOpts)
	if err != nil {
		return nil, err
	}

	finish := func() {
		finished()
		closer.Close()
		conn.Close()
	}

	return &GitalyConn{ctx: ctx, conn: conn, close: finish}, nil
}
