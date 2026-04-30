//go:build acceptance

package acceptancetest

import "time"

// Result is what Run returns: the exit code, captured stdout/stderr, and
// wall-clock duration of the spawned binary.
type Result struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
}
