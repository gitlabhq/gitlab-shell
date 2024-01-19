//go:build !gssapi

package sshd

import (
	"errors"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
)

func NewGSSAPIServer(c *config.GSSAPIConfig) (*OSGSSAPIServer, error) {
	s := &OSGSSAPIServer{
		ServicePrincipalName: c.ServicePrincipalName,
	}

	return s, nil
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
