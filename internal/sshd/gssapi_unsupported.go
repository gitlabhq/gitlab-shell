//go:build !cgo

package sshd

import (
	"errors"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"

	"gitlab.com/gitlab-org/labkit/log"
)

func LoadGSSAPILib(c *config.GSSAPIConfig) error {
	if c.Enabled {
		log.New().Error("gssapi-with-mic disabled, built without CGO")
		c.Enabled = false
	}
	return nil
}

type OSGSSAPIServer struct {
	ServicePrincipalName string
}

func (*OSGSSAPIServer) AcceptSecContext([]byte) ([]byte, string, bool, error) {
	return []byte{}, "", false, errors.New("gssapi is unsupported")
}

func (*OSGSSAPIServer) VerifyMIC([]byte, []byte) error {
	return errors.New("gssapi is unsupported")
}
func (*OSGSSAPIServer) DeleteSecContext() error {
	return errors.New("gssapi is unsupported")
}
