package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/config"
)

func makeHandler(t *testing.T, err error) func(context.Context, *grpc.ClientConn) (int32, error) {
	return func(ctx context.Context, client *grpc.ClientConn) (int32, error) {
		require.NotNil(t, ctx)
		require.NotNil(t, client)

		return 0, err
	}
}

func TestRunGitalyCommand(t *testing.T) {
	cmd := GitalyCommand{
		Config:  &config.Config{},
		Address: "tcp://localhost:9999",
	}

	err := cmd.RunGitalyCommand(makeHandler(t, nil))
	require.NoError(t, err)

	expectedErr := errors.New("error")
	err = cmd.RunGitalyCommand(makeHandler(t, expectedErr))
	require.Equal(t, err, expectedErr)
}

func TestMissingGitalyAddress(t *testing.T) {
	cmd := GitalyCommand{Config: &config.Config{}}

	err := cmd.RunGitalyCommand(makeHandler(t, nil))
	require.EqualError(t, err, "no gitaly_address given")
}
