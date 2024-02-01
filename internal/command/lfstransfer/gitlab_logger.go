// Package lfstransfer wraps https://github.com/charmbracelet/git-lfs-transfer logic
package lfstransfer

import (
	"context"

	"gitlab.com/gitlab-org/labkit/log"
)

// WrappedLoggerForGitLFSTransfer is responsible for creating a compatible logger
// for github.com/charmbracelet/git-lfs-transfer
type WrappedLoggerForGitLFSTransfer struct {
	ctx context.Context
}

// NewWrappedLoggerForGitLFSTransfer returns a new WrappedLoggerForGitLFSTransfer
// passing through context.Context
func NewWrappedLoggerForGitLFSTransfer(ctx context.Context) *WrappedLoggerForGitLFSTransfer {
	return &WrappedLoggerForGitLFSTransfer{ctx: ctx}
}

// Log allows logging in github.com/charmbracelet/git-lfs-transfer to take place
// using gitlab.com/gitlab-org/labkit/log
func (l *WrappedLoggerForGitLFSTransfer) Log(msg string, args ...interface{}) {
	fields := make(map[string]interface{})
	fieldsFallback := map[string]interface{}{"args": args}

	for i := 0; i < len(args); i += 2 {
		if i >= len(args)-1 {
			fields = fieldsFallback
			break
		}

		if arg, ok := args[i].(string); ok {
			fields[arg] = args[i+1]
		} else {
			fields = fieldsFallback
			break
		}
	}

	log.WithContextFields(l.ctx, fields).Info(msg)
}
