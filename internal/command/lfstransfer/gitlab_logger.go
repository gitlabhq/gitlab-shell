package lfstransfer

import (
	"context"

	"gitlab.com/gitlab-org/labkit/log"
)

type GitlabLogger struct {
	ctx context.Context
}

func NewGitlabLogger(ctx context.Context) *GitlabLogger {
	return &GitlabLogger{ctx: ctx}
}

func (l *GitlabLogger) Log(msg string, args ...interface{}) {
	fields := make(map[string]interface{})
	fieldsFallback := map[string]interface{}{"args": args}

	for i := 0; i < len(args); i += 2 {
		if i+1 >= len(args) {
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
