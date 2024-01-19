//go:build gssapi

package sshd

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
)

func NewGSSAPIServerSuccess(t *testing.T) {
	config := &config.GSSAPIConfig{Enabled: true, ServicePrincipalName: "host/test@TEST.TEST"}
	s, err := NewGSSAPIServer(config)

	require.NotNil(t, s)
	require.NotNil(t, s.lib)
	require.Nil(t, err)
	require.True(t, config.Enabled)
}

func NewGSSAPIServerFailure(t *testing.T) {
	config := &config.GSSAPIConfig{Enabled: true, LibPath: "/invalid", ServicePrincipalName: "host/test@TEST.TEST"}
	s, err := NewGSSAPIServer(config)

	require.Nil(t, s)
	require.NotNil(t, err)
	require.False(t, config.Enabled)
}
