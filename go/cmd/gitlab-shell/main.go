package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"gitlab.com/gitlab-org/gitlab-shell/go/cmd/gitlab-shell/command"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/config"
)

var (
	binDir   string
	rootDir  string
	features map[command.CommandType]bool
)

func init() {
	binDir = filepath.Dir(os.Args[0])
	rootDir = filepath.Dir(binDir)
	features = map[command.CommandType]bool{}
}

func migrate(config *config.Config) (int, bool) {
	if !config.Migration.Enabled {
		return 0, false
	}
	command, err := command.New(os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to build command: %v\n", err)
		return 0, false
	}

	if featureEnabled(config, command.Type) {

	}

	return 0, false
}

// rubyExec will never return. It either replaces the current process with a
// Ruby interpreter, or outputs an error and kills the process.
func execRuby() {
	rubyCmd := filepath.Join(binDir, "gitlab-shell-ruby")

	execErr := syscall.Exec(rubyCmd, os.Args, os.Environ())
	if execErr != nil {
		fmt.Fprintf(os.Stderr, "Failed to exec(%q): %v\n", rubyCmd, execErr)
		os.Exit(1)
	}
}

func main() {
	// Fall back to Ruby in case of problems reading the config, but issue a
	// warning as this isn't something we can sustain indefinitely
	config, err := config.NewFromDir(rootDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read config, falling back to gitlab-shell-ruby: %v", err)
		execRuby()
	}

	// Try to handle the command with the Go implementation
	if exitCode, done := migrate(config); done {
		os.Exit(exitCode)
	}

	// Since a migration has not handled the command, fall back to Ruby to do so
	execRuby()
}

func featureEnabled(config *config.Config, commandType command.CommandType) bool {
	if features[commandType] {
		return true
	}

	fmt.Fprintf(os.Stderr, "Config: %v", config.Migration)

	for _, featureName := range config.Migration.Features {
		fmt.Fprintf(os.Stderr, "Setting feature: %v to %v", featureName, command.CommandType(featureName))

		features[command.CommandType(featureName)] = true
	}

	return features[commandType]
}
