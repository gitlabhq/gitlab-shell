package handler

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/testhelper"
	"google.golang.org/grpc"
)

func TestInteralRunHandler(t *testing.T) {
	type testCase struct {
		name    string
		args    []string
		handler func(context.Context, *grpc.ClientConn, string) (int32, error)
		want    int
		wantErr bool
	}

	var currentTest *testCase
	makeHandler := func(r1 int32, r2 error) func(context.Context, *grpc.ClientConn, string) (int32, error) {
		return func(ctx context.Context, client *grpc.ClientConn, requestJSON string) (int32, error) {
			require.NotNil(t, ctx)
			require.NotNil(t, client)
			require.Equal(t, currentTest.args[2], requestJSON)
			return r1, r2
		}
	}
	tests := []testCase{
		{
			name:    "expected",
			args:    []string{"test", "tcp://localhost:9999", "{}"},
			handler: makeHandler(0, nil),
			want:    0,
			wantErr: false,
		},
		{
			name:    "handler_error",
			args:    []string{"test", "tcp://localhost:9999", "{}"},
			handler: makeHandler(0, fmt.Errorf("error")),
			want:    0,
			wantErr: true,
		},
		{
			name:    "handler_exitcode",
			args:    []string{"test", "tcp://localhost:9999", "{}"},
			handler: makeHandler(1, nil),
			want:    1,
			wantErr: false,
		},
		{
			name:    "handler_error_exitcode",
			args:    []string{"test", "tcp://localhost:9999", "{}"},
			handler: makeHandler(1, fmt.Errorf("error")),
			want:    1,
			wantErr: true,
		},
		{
			name:    "too_few_arguments",
			args:    []string{"test"},
			handler: makeHandler(10, nil),
			want:    1,
			wantErr: true,
		},
		{
			name:    "too_many_arguments",
			args:    []string{"test", "1", "2", "3"},
			handler: makeHandler(10, nil),
			want:    1,
			wantErr: true,
		},
		{
			name:    "empty_gitaly_address",
			args:    []string{"test", "", "{}"},
			handler: makeHandler(10, nil),
			want:    1,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			currentTest = &tt
			defer func() {
				currentTest = nil
			}()

			done, err := testhelper.PrepareTestRootDir()
			defer done()
			require.NoError(t, err)

			got, err := internalRunGitalyCommand(tt.args, tt.handler)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, tt.want, got)
		})
	}
}
