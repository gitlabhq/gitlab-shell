//go:build cgo

package sshd

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
)

func TestLoadGSSAPILibSucces(t *testing.T) {
	config := &config.GSSAPIConfig{Enabled: true}
	err := LoadGSSAPILib(config)

	require.NotNil(t, lib)
	require.Nil(t, err)
	require.True(t, config.Enabled)
}

func TestLoadGSSAPILibFailure(t *testing.T) {
	config := &config.GSSAPIConfig{Enabled: true, LibPath: "/invalid"}
	err := LoadGSSAPILib(config)

	require.Nil(t, lib)
	require.NotNil(t, err)
	require.False(t, config.Enabled)
}
