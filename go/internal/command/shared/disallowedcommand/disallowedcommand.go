package disallowedcommand

import "errors"

var (
	Error = errors.New("> GitLab: Disallowed command")
)
