package handler

import (
	"context"
	"fmt"
	"os"
	"strings"

	"gitlab.com/gitlab-org/gitaly/auth"
	"gitlab.com/gitlab-org/gitaly/client"

	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/labkit/tracing"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

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
	Features    map[string]string
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

func withOutgoingMetadata(ctx context.Context, features map[string]string) context.Context {
	md := metadata.New(nil)
	for k, v := range features {
		if !strings.HasPrefix(k, "gitaly-feature-") {
			continue
		}
		md.Append(k, v)
	}

	return metadata.NewOutgoingContext(ctx, md)
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
	ctx = withOutgoingMetadata(ctx, gc.Features)

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
