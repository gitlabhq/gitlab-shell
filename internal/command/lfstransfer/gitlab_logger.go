// Package lfstransfer wraps https://github.com/charmbracelet/git-lfs-transfer logic
package lfstransfer

import (
	"context"
	"log/slog"

	"gitlab.com/gitlab-org/labkit/v2/log"
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
// using gitlab.com/gitlab-org/labkit/v2/log
func (l *WrappedLoggerForGitLFSTransfer) Log(msg string, args ...any) {
	fields := []slog.Attr{}
	fieldsFallback := []slog.Attr{slog.Any("args", args)}

	for i := 0; i < len(args); i += 2 {
		if i >= len(args)-1 {
			fields = fieldsFallback
			break
		}

		if arg, ok := args[i].(string); ok {
			fields = append(fields, slog.Any(arg, args[i+1]))
		} else {
			fields = fieldsFallback
			break
		}
	}
	ctx := log.WithFields(l.ctx, fields...)
	slog.InfoContext(ctx, msg)
}
