package main

import (
	"testing"

	"github.com/stretchr/testify/require"
	pb "gitlab.com/gitlab-org/gitaly/proto/go/gitalypb"
)

func Test_deserialize(t *testing.T) {
	tests := []struct {
		name        string
		requestJSON string
		want        *pb.SSHUploadPackRequest
		wantErr     bool
	}{
		{
			name:        "empty",
			requestJSON: "",
			want:        nil,
			wantErr:     true,
		},
		{
			name:        "empty_hash",
			requestJSON: "{}",
			want:        &pb.SSHUploadPackRequest{},
			wantErr:     false,
		},
		{
			name:        "nil",
			requestJSON: "null",
			want:        &pb.SSHUploadPackRequest{},
			wantErr:     false,
		},
		{
			name:        "values",
			requestJSON: `{"repository": { "storage_name": "12345"} }`,
			want:        &pb.SSHUploadPackRequest{Repository: &pb.Repository{StorageName: "12345"}},
			wantErr:     false,
		},
		{
			name:        "invalid_json",
			requestJSON: `{"gl_id": "1234`,
			want:        nil,
			wantErr:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := deserialize(tt.requestJSON)
			require.EqualValues(t, got, tt.want, "Got %+v, wanted %+v", got, tt.want)
			if tt.wantErr {
				require.Error(t, err, "Wanted an error, got %+v", err)
			} else {
				require.NoError(t, err, "Wanted no error, got %+v", err)
			}
		})
	}
}
