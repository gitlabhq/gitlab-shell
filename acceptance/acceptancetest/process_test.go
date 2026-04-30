//go:build acceptance

package acceptancetest

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRunBinary_capturesStdoutStderrAndExitCode(t *testing.T) {
	res := runBinary(t, runBinaryArgs{
		Path: "/bin/sh",
		Args: []string{"-c", "echo out; echo err 1>&2; exit 7"},
	})

	require.Equal(t, 7, res.ExitCode)
	require.Equal(t, "out\n", res.Stdout)
	require.Equal(t, "err\n", res.Stderr)
	require.NotZero(t, res.Duration)
}

func TestRunBinary_zeroExitCode(t *testing.T) {
	res := runBinary(t, runBinaryArgs{Path: "/bin/sh", Args: []string{"-c", "true"}})
	require.Equal(t, 0, res.ExitCode)
}

func TestRunBinary_timeoutKillsProcess(t *testing.T) {
	res := runBinary(t, runBinaryArgs{
		Path:    "/bin/sh",
		Args:    []string{"-c", "sleep 30"},
		Timeout: 200 * time.Millisecond,
	})

	require.NotEqual(t, 0, res.ExitCode, "expected non-zero from killed process")
	require.Less(t, res.Duration, 5*time.Second, "expected SIGTERM/KILL to land quickly")
}

func TestRunBinary_passesEnvAndStdin(t *testing.T) {
	res := runBinary(t, runBinaryArgs{
		Path:  "/bin/sh",
		Args:  []string{"-c", "read line; echo \"$line $MY_VAR\""},
		Stdin: strings.NewReader("hello\n"),
		Env:   []string{"MY_VAR=world"},
	})

	require.Equal(t, 0, res.ExitCode, "stderr: %s", res.Stderr)
	require.Equal(t, "hello world\n", res.Stdout)
}
