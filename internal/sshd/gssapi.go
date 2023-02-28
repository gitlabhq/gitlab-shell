//go:build cgo

package sshd

import (
	"fmt"

	"github.com/openshift/gssapi"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"

	"gitlab.com/gitlab-org/labkit/log"
)

var lib *gssapi.Lib

func LoadGSSAPILib(config *config.GSSAPIConfig) error {
	var err error

	if config.Enabled {
		options := &gssapi.Options{
			Krb5Ktname: config.Keytab,
		}

		if config.LibPath != "" {
			options.LibPath = config.LibPath
		}

		lib, err = gssapi.Load(options)

		if err != nil {
			log.WithError(err).Error("Unable to load GSSAPI library, gssapi-with-mic is disabled")
			config.Enabled = false
		}
	}

	return err
}

type OSGSSAPIServer struct {
	Keytab               string
	ServicePrincipalName string

	contextId *gssapi.CtxId
}

func (_ *OSGSSAPIServer) str2name(str string) (*gssapi.Name, error) {
	strBuffer, err := lib.MakeBufferString(str)
	if err != nil {
		return nil, err
	}
	defer strBuffer.Release()

	return strBuffer.Name(lib.GSS_C_NO_OID)
}

func (server *OSGSSAPIServer) AcceptSecContext(
	token []byte,
) (
	outputToken []byte,
	srcName string,
	needContinue bool,
	err error,
) {
	tokenBuffer, err := lib.MakeBufferBytes(token)
	if err != nil {
		return
	}
	defer tokenBuffer.Release()

	var spn *gssapi.CredId = lib.GSS_C_NO_CREDENTIAL
	if server.ServicePrincipalName != "" {
		var name *gssapi.Name
		name, err = server.str2name(server.ServicePrincipalName)
		if err != nil {
			return
		}
		defer name.Release()

		var actualMech *gssapi.OIDSet
		spn, actualMech, _, err = lib.AcquireCred(name, 0, lib.GSS_C_NO_OID_SET, gssapi.GSS_C_ACCEPT)
		if err != nil {
			return
		}
		defer spn.Release()
		defer actualMech.Release()
	}

	ctxOut, srcNameName, _, outputTokenBuffer, _, _, _, err := lib.AcceptSecContext(
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
	if server.contextId == nil {
		return fmt.Errorf("gssapi: uninitialized contextId")
	}

	micFieldBuffer, err := lib.MakeBufferBytes(micField)
	if err != nil {
		return err
	}
	defer micFieldBuffer.Release()
	micTokenBuffer, err := lib.MakeBufferBytes(micToken)
	if err != nil {
		return err
	}
	defer micTokenBuffer.Release()

	_, err = server.contextId.VerifyMIC(micFieldBuffer, micTokenBuffer)
	return err

}

func (server *OSGSSAPIServer) DeleteSecContext() error {
	if server.contextId == nil {
		return nil
	}

	err := server.contextId.DeleteSecContext()
	if err == nil {
		server.contextId = nil
	}
	return err
}
