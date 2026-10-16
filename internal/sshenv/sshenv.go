// Package sshenv provides functionality for handling SSH environment variables
package sshenv

import (
	"os"
	"strings"
)

const (
	// GitProtocolEnv defines the ENV name holding the git protocol used
	GitProtocolEnv = "GIT_PROTOCOL"
	// SSHConnectionEnv defines the ENV holding the SSH connection
	SSHConnectionEnv = "SSH_CONNECTION"
	// SSHOriginalCommandEnv defines the ENV containing the original SSH command
	SSHOriginalCommandEnv = "SSH_ORIGINAL_COMMAND"
)

// Env represents the SSH environment variables
type Env struct {
	GitProtocolVersion string
	IsSSHConnection    bool
	OriginalCommand    string
	RemoteAddr         string
	NamespacePath      string
}

// NewFromEnv creates a new Env instance based on the current environment variables
func NewFromEnv() Env {
	remoteAddr := remoteAddrFromEnv()

	return Env{
		GitProtocolVersion: os.Getenv(GitProtocolEnv),
		IsSSHConnection:    remoteAddr != "",
		RemoteAddr:         remoteAddr,
		OriginalCommand:    os.Getenv(SSHOriginalCommandEnv),
	}
}

// remoteAddrFromEnv returns the connection address from ENV string
func remoteAddrFromEnv() string {
	fields := strings.Fields(os.Getenv(SSHConnectionEnv))

	if len(fields) > 0 {
		return fields[0]
	}
	return ""
}
