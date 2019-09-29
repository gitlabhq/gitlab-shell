package sshenv

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/testhelper"
)

func TestLocalAddr(t *testing.T) {
	cleanup, err := testhelper.Setenv("SSH_CONNECTION", "127.0.0.1 0")
	require.NoError(t, err)
	defer cleanup()

	require.Nil(t, err)
	require.Equal(t, LocalAddr(), "127.0.0.1")
}
