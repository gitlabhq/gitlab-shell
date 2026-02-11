//go:build gssapi

package sshd

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/openshift/gssapi"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/labkit/fields"
)

func NewGSSAPIServer(c *config.GSSAPIConfig) (*OSGSSAPIServer, error) {
	lib, err := loadGSSAPILib(c)
	if err != nil {
		return nil, err
	}

	s := &OSGSSAPIServer{
		ServicePrincipalName: c.ServicePrincipalName,
		lib:                  lib,
	}

	return s, nil
}

func loadGSSAPILib(config *config.GSSAPIConfig) (*gssapi.Lib, error) {
	var err error
	var lib *gssapi.Lib

	if config.Enabled {
		options := &gssapi.Options{
			Krb5Ktname: config.Keytab,
		}

		if config.LibPath != "" {
			options.LibPath = config.LibPath
		}

		lib, err = gssapi.Load(options)

		if err != nil {
			slog.Error("Unable to load GSSAPI library, gssapi-with-mic is disabled", slog.String(fields.ErrorMessage, err.Error()))
			config.Enabled = false
		}
	}

	return lib, err
}

type OSGSSAPIServer struct {
	Keytab               string
	ServicePrincipalName string

	mutex     sync.RWMutex
	lib       *gssapi.Lib
	contextId *gssapi.CtxId
}

func (server *OSGSSAPIServer) str2name(str string) (*gssapi.Name, error) {
	strBuffer, err := server.lib.MakeBufferString(str)
	if err != nil {
		return nil, err
	}
	defer strBuffer.Release()

	return strBuffer.Name(server.lib.GSS_C_NO_OID)
}

func (server *OSGSSAPIServer) AcceptSecContext(
	token []byte,
) (
	outputToken []byte,
	srcName string,
	needContinue bool,
	err error,
) {
	server.mutex.Lock()
	defer server.mutex.Unlock()

	tokenBuffer, err := server.lib.MakeBufferBytes(token)
	if err != nil {
		return
	}
	defer tokenBuffer.Release()

	var spn *gssapi.CredId = server.lib.GSS_C_NO_CREDENTIAL
	if server.ServicePrincipalName != "" {
		var name *gssapi.Name
		name, err = server.str2name(server.ServicePrincipalName)
		if err != nil {
			return
		}
		defer name.Release()

		var actualMech *gssapi.OIDSet
		spn, actualMech, _, err = server.lib.AcquireCred(name, 0, server.lib.GSS_C_NO_OID_SET, gssapi.GSS_C_ACCEPT)
		if err != nil {
			return
		}
		defer spn.Release()
		defer actualMech.Release()
	}

	ctxOut, srcNameName, _, outputTokenBuffer, _, _, _, err := server.lib.AcceptSecContext(
		server.contextId,
		spn,
		tokenBuffer,
		nil,
	)
	if err == gssapi.ErrContinueNeeded {
		needContinue = true
		err = nil
	} else if err != nil {
		return
	}
	defer outputTokenBuffer.Release()
	defer srcNameName.Release()

	outputToken = outputTokenBuffer.Bytes()
	server.contextId = ctxOut

	return outputToken, srcNameName.String(), needContinue, err
}

func (server *OSGSSAPIServer) VerifyMIC(
	micField []byte,
	micToken []byte,
) error {
	server.mutex.Lock()
	defer server.mutex.Unlock()

	if server.contextId == nil {
		return fmt.Errorf("gssapi: uninitialized contextId")
	}

	micFieldBuffer, err := server.lib.MakeBufferBytes(micField)
	if err != nil {
		return err
	}
	defer micFieldBuffer.Release()
	micTokenBuffer, err := server.lib.MakeBufferBytes(micToken)
	if err != nil {
		return err
	}
	defer micTokenBuffer.Release()

	_, err = server.contextId.VerifyMIC(micFieldBuffer, micTokenBuffer)
	return err

}

func (server *OSGSSAPIServer) DeleteSecContext() error {
	server.mutex.Lock()
	defer server.mutex.Unlock()

	if server.contextId == nil {
		return nil
	}

	err := server.contextId.DeleteSecContext()
	if err == nil {
		server.contextId = server.lib.GSS_C_NO_CONTEXT
	}
	return err
}
