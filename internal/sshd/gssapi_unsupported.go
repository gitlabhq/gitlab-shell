//go:build !gssapi

package sshd

import (
	"errors"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
)

// NewGSSAPIServer initializes and returns a new OSGSSAPIServer.
func NewGSSAPIServer(c *config.GSSAPIConfig) (*OSGSSAPIServer, error) {
	s := &OSGSSAPIServer{
		ServicePrincipalName: c.ServicePrincipalName,
	}

	return s, nil
}

// OSGSSAPIServer represents a server that handles GSSAPI requests.
type OSGSSAPIServer struct {
	ServicePrincipalName string
}

// AcceptSecContext returns an error indicating that GSSAPI is unsupported.
func (*OSGSSAPIServer) AcceptSecContext([]byte) ([]byte, string, bool, error) {
	return []byte{}, "", false, errors.New("gssapi is unsupported")
}

// VerifyMIC returns an error indicating that GSSAPI is unsupported.
func (*OSGSSAPIServer) VerifyMIC([]byte, []byte) error {
	return errors.New("gssapi is unsupported")
}

// DeleteSecContext returns an error indicating that GSSAPI is unsupported.
func (*OSGSSAPIServer) DeleteSecContext() error {
	return errors.New("gssapi is unsupported")
}
