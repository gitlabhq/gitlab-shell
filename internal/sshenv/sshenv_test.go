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
			restoreEnv := testhelper.TempEnv(tc.environment)
			defer restoreEnv()

			require.Equal(t, NewFromEnv(), tc.want)
		})
	}
}

func TestRemoteAddrFromEnv(t *testing.T) {
	cleanup, err := testhelper.Setenv(SSHConnectionEnv, "127.0.0.1 0")
	require.NoError(t, err)
	defer cleanup()

	require.Equal(t, remoteAddrFromEnv(), "127.0.0.1")
}

func TestEmptyRemoteAddrFromEnv(t *testing.T) {
	require.Equal(t, remoteAddrFromEnv(), "")
}
