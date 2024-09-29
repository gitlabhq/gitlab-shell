// Package commandargs defines types and interfaces for handling command-line arguments
// in GitLab shell commands.
package commandargs

// CommandType represents a type of command identified by a string.
type CommandType string

// CommandArgs is an interface for parsing and accessing command-line arguments.
type CommandArgs interface {
	Parse() error
	GetArguments() []string
}
