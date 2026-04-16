package sshenv

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/testhelper"
)

func TestNewFromEnv(t *testing.T) {
	tests := []struct {
		desc        string
		environment map[string]string
		want        Env
	}{
		{
			desc:        "It parses GIT_PROTOCOL",
			environment: map[string]string{GitProtocolEnv: "2"},
			want:        Env{GitProtocolVersion: "2"},
		},
		{
			desc:        "It parses SSH_CONNECTION",
			environment: map[string]string{SSHConnectionEnv: "127.0.0.1 0 127.0.0.2 65535"},
			want:        Env{IsSSHConnection: true, RemoteAddr: "127.0.0.1"},
		},
		{
			desc:        "It parses SSH_ORIGINAL_COMMAND",
			environment: map[string]string{SSHOriginalCommandEnv: "git-receive-pack"},
			want:        Env{OriginalCommand: "git-receive-pack"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			testhelper.TempEnv(t, tc.environment)

			require.Equal(t, tc.want, NewFromEnv())
		})
	}
}

func TestRemoteAddrFromEnv(t *testing.T) {
	t.Setenv(SSHConnectionEnv, "127.0.0.1 0")

	require.Equal(t, "127.0.0.1", remoteAddrFromEnv())
}

func TestEmptyRemoteAddrFromEnv(t *testing.T) {
	require.Empty(t, remoteAddrFromEnv())
}
