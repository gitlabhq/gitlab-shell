package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	pb "gitlab.com/gitlab-org/gitaly/proto/go/gitalypb"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/internal/testhelper"
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
				require.Equal(t, 1, len(values))
				require.Equal(t, v, values[0])
			}

		})
	}
}

func TestPrepareContext(t *testing.T) {
	tests := []struct {
		name             string
		gc               *GitalyCommand
		sshConnectionEnv string
		repo             *pb.Repository
		response         *accessverifier.Response
		want             map[string]string
	}{
		{
			name: "client_identity",
			gc: &GitalyCommand{
				Config:  &config.Config{},
				Address: "tcp://localhost:9999",
			},
			sshConnectionEnv: "10.0.0.1 1234 127.0.0.1 5678",
			repo: &pb.Repository{
				StorageName:                   "default",
				RelativePath:                  "@hashed/5f/9c/5f9c4ab08cac7457e9111a30e4664920607ea2c115a1433d7be98e97e64244ca.git",
				GitObjectDirectory:            "path/to/git_object_directory",
				GitAlternateObjectDirectories: []string{"path/to/git_alternate_object_directory"},
				GlRepository:                  "project-26",
				GlProjectPath:                 "group/private",
			},
			response: &accessverifier.Response{
				UserId:   "6",
				Username: "jane.doe",
			},
			want: map[string]string{
				"remote_ip": "10.0.0.1",
				"user_id":   "6",
				"username":  "jane.doe",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup, err := testhelper.Setenv("SSH_CONNECTION", tt.sshConnectionEnv)
			require.NoError(t, err)
			defer cleanup()

			ctx := context.Background()

			ctx, cancel := tt.gc.PrepareContext(ctx, tt.repo, tt.response, "protocol")
			defer cancel()

			md, exists := metadata.FromOutgoingContext(ctx)
			require.True(t, exists)
			require.Equal(t, len(tt.want), md.Len())

			for k, v := range tt.want {
				values := md.Get(k)
				require.Equal(t, 1, len(values))
				require.Equal(t, v, values[0])
			}

		})
	}
}
