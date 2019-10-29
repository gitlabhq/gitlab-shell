package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
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

func TestGetConnMetadata(t *testing.T) {
	tests := []struct {
		name string
		gc   *GitalyCommand
		want map[string]string
	}{
		{
			name: "gitaly_feature_flags",
			gc: &GitalyCommand{
				Config:  &config.Config{},
				Address: "tcp://localhost:9999",
				Features: map[string]string{
					"gitaly-feature-cache_invalidator":        "true",
					"other-ff":                                "true",
					"gitaly-feature-inforef_uploadpack_cache": "false",
				},
			},
			want: map[string]string{
				"gitaly-feature-cache_invalidator":        "true",
				"gitaly-feature-inforef_uploadpack_cache": "false",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn, err := getConn(tt.gc)
			require.NoError(t, err)

			md, exists := metadata.FromOutgoingContext(conn.ctx)
			require.True(t, exists)
			require.Equal(t, len(tt.want), md.Len())

			for k, v := range tt.want {
				values := md.Get(k)
				assert.Equal(t, 1, len(values))
				assert.Equal(t, v, values[0])
			}

		})
	}
}
