//go:build acceptance

package acceptancetest

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"
)

const defaultRunTimeout = 30 * time.Second

// runBinaryArgs is the internal shape passed to runBinary. Public callers
// go through Run/Config.
type runBinaryArgs struct {
	Path    string
	Args    []string
	Env     []string // already merged; no merging happens here
	Stdin   io.Reader
	Timeout time.Duration // 0 = defaultRunTimeout
}

// runBinary spawns Path with Args, captures stdout/stderr to in-memory
// buffers, applies a timeout, and returns Result. A non-zero exit (whether
// from the binary or from a timeout-kill) is reported via Result.ExitCode
// and is NOT a test failure — that's the caller's call. Spawning failures
// (binary not found, fork failure) call t.Fatalf because they aren't
// behaviour of the binary under test.
func runBinary(t *testing.T, args runBinaryArgs) Result {
	t.Helper()

	timeout := args.Timeout
	if timeout == 0 {
		timeout = defaultRunTimeout
	}

	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	defer cancel()

	var stdout, stderr bytes.Buffer

	cmd := exec.CommandContext(ctx, args.Path, args.Args...)
	cmd.Env = args.Env
	cmd.Stdin = args.Stdin
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 2 * time.Second

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	result := Result{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: duration,
	}

	if err == nil {
		result.ExitCode = 0
		return result
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result
	}

	t.Fatalf("runBinary(%q): unexpected error: %v\nstderr: %s", args.Path, err, stderr.String())
	return result
}
