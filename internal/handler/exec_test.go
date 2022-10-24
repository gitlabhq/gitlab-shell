package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	grpccodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	grpcstatus "google.golang.org/grpc/status"

	pb "gitlab.com/gitlab-org/gitaly/v15/proto/go/gitalypb"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/sshenv"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
)

func makeHandler(t *testing.T, err error) func(context.Context, *grpc.ClientConn) (int32, error) {
	return func(ctx context.Context, client *grpc.ClientConn) (int32, error) {
		require.NotNil(t, ctx)
		require.NotNil(t, client)

		return 0, err
	}
}

func TestRunGitalyCommand(t *testing.T) {
	cmd := NewGitalyCommand(
		newConfig(),
		string(commandargs.UploadPack),
		&accessverifier.Response{
			Gitaly: accessverifier.Gitaly{Address: "tcp://localhost:9999"},
		},
	)

	err := cmd.RunGitalyCommand(context.Background(), makeHandler(t, nil))
	require.NoError(t, err)

	expectedErr := errors.New("error")
	err = cmd.RunGitalyCommand(context.Background(), makeHandler(t, expectedErr))
	require.Equal(t, err, expectedErr)
}

func TestCachingOfGitalyConnections(t *testing.T) {
	ctx := context.Background()
	cfg := newConfig()
	response := &accessverifier.Response{
		Username: "user",
		Gitaly: accessverifier.Gitaly{
			Address: "tcp://localhost:9999",
			Token:   "token",
		},
	}

	cmd := NewGitalyCommand(cfg, string(commandargs.UploadPack), response)

	conn, err := cmd.getConn(ctx)
	require.NoError(t, err)

	// Reuses connection for different users
	response.Username = "another-user"
	cmd = NewGitalyCommand(cfg, string(commandargs.UploadPack), response)
	newConn, err := cmd.getConn(ctx)
	require.NoError(t, err)
	require.Equal(t, conn, newConn)
}

func TestMissingGitalyAddress(t *testing.T) {
	cmd := GitalyCommand{Config: newConfig()}

	err := cmd.RunGitalyCommand(context.Background(), makeHandler(t, nil))
	require.EqualError(t, err, "no gitaly_address given")
}

func TestUnavailableGitalyErr(t *testing.T) {
	cmd := NewGitalyCommand(
		newConfig(),
		string(commandargs.UploadPack),
		&accessverifier.Response{
			Gitaly: accessverifier.Gitaly{Address: "tcp://localhost:9999"},
		},
	)

	err := cmd.RunGitalyCommand(context.Background(), makeHandler(t, grpcstatus.Error(grpccodes.Unavailable, "error")))
	require.Equal(t, err, grpcstatus.Error(grpccodes.Unavailable, "The git server, Gitaly, is not available at this time. Please contact your administrator."))
}

func TestGitalyLimitErr(t *testing.T) {
	cmd := NewGitalyCommand(
		newConfig(),
		string(commandargs.UploadPack),
		&accessverifier.Response{
			Gitaly: accessverifier.Gitaly{Address: "tcp://localhost:9999"},
		},
	)
	limitErr := errWithDetail(t, &pb.LimitError{
		ErrorMessage: "concurrency queue wait time reached",
		RetryAfter:   durationpb.New(0)})
	err := cmd.RunGitalyCommand(context.Background(), makeHandler(t, limitErr))
	require.Equal(t, err, grpcstatus.Error(grpccodes.Unavailable, "GitLab is currently unable to handle this request due to load."))
}

func TestRunGitalyCommandMetadata(t *testing.T) {
	tests := []struct {
		name string
		gc   *GitalyCommand
		want map[string]string
	}{
		{
			name: "gitaly_feature_flags",
			gc: NewGitalyCommand(
				newConfig(),
				string(commandargs.UploadPack),
				&accessverifier.Response{
					Gitaly: accessverifier.Gitaly{
						Address: "tcp://localhost:9999",
						Features: map[string]string{
							"gitaly-feature-cache_invalidator":        "true",
							"other-ff":                                "true",
							"gitaly-feature-inforef_uploadpack_cache": "false",
						},
					},
				},
			),
			want: map[string]string{
				"gitaly-feature-cache_invalidator":        "true",
				"gitaly-feature-inforef_uploadpack_cache": "false",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.gc

			err := cmd.RunGitalyCommand(context.Background(), func(ctx context.Context, _ *grpc.ClientConn) (int32, error) {
				md, exists := metadata.FromOutgoingContext(ctx)
				require.True(t, exists)
				require.Equal(t, len(tt.want), md.Len())

				for k, v := range tt.want {
					values := md.Get(k)
					require.Equal(t, 1, len(values))
					require.Equal(t, v, values[0])
				}

				return 0, nil
			})

			require.NoError(t, err)
		})
	}
}

func TestPrepareContext(t *testing.T) {
	tests := []struct {
		name     string
		gc       *GitalyCommand
		env      sshenv.Env
		repo     *pb.Repository
		response *accessverifier.Response
		want     map[string]string
	}{
		{
			name: "client_identity",
			gc: NewGitalyCommand(
				&config.Config{},
				string(commandargs.UploadPack),
				&accessverifier.Response{
					KeyId:    1,
					KeyType:  "key",
					UserId:   "6",
					Username: "jane.doe",
					Gitaly: accessverifier.Gitaly{
						Address: "tcp://localhost:9999",
					},
				},
			),
			env: sshenv.Env{
				GitProtocolVersion: "protocol",
				IsSSHConnection:    true,
				RemoteAddr:         "10.0.0.1",
			},
			repo: &pb.Repository{
				StorageName:                   "default",
				RelativePath:                  "@hashed/5f/9c/5f9c4ab08cac7457e9111a30e4664920607ea2c115a1433d7be98e97e64244ca.git",
				GitObjectDirectory:            "path/to/git_object_directory",
				GitAlternateObjectDirectories: []string{"path/to/git_alternate_object_directory"},
				GlRepository:                  "project-26",
				GlProjectPath:                 "group/private",
			},
			want: map[string]string{
				"key_id":    "1",
				"key_type":  "key",
				"user_id":   "6",
				"username":  "jane.doe",
				"remote_ip": "10.0.0.1",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			ctx, cancel := tt.gc.PrepareContext(ctx, tt.repo, tt.env)
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

func newConfig() *config.Config {
	cfg := &config.Config{}
	cfg.GitalyClient.InitSidechannelRegistry(context.Background())
	return cfg
}

// errWithDetail adds the given details to the error if it is a gRPC status whose code is not OK.
func errWithDetail(t *testing.T, detail proto.Message) error {
	st := grpcstatus.New(grpccodes.Unavailable, "too busy")

	proto := st.Proto()
	marshaled, err := anypb.New(detail)
	require.NoError(t, err)

	proto.Details = append(proto.Details, marshaled)

	return grpcstatus.ErrorProto(proto)
}
