// Package disallowedcommand provides an error for handling disallowed commands.
package disallowedcommand

import "errors"

var (
	// Error is returned when a disallowed command is encountered.
	Error = errors.New("Disallowed command") //nolint:staticcheck // message is customer facing
)
