package sshenv

import (
	"os"
	"strings"
)

func LocalAddr() string {
	address := os.Getenv("SSH_CONNECTION")

	if address != "" {
		return strings.Fields(address)[0]
	}
	return ""
}
