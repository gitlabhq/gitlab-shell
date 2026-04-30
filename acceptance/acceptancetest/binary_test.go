//go:build acceptance

package acceptancetest

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildBinary_returnsExecutablePath(t *testing.T) {
	path := BuildBinary(t, "gitlab-shell-check")

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.False(t, info.IsDir(), "expected file, got dir at %s", path)
	require.NotZero(t, info.Mode()&0o111, "binary at %s is not executable", path)
}

func TestBuildBinary_isCachedPerName(t *testing.T) {
	first := BuildBinary(t, "gitlab-shell-check")
	second := BuildBinary(t, "gitlab-shell-check")

	require.Equal(t, first, second, "expected cached path on second call")
}

func TestBuildBinary_unknownBinaryFailsTest(t *testing.T) {
	tt := &subTestRecorder{T: t}
	defer func() {
		_ = recover()
	}()

	BuildBinary(tt, "this-binary-does-not-exist")

	require.True(t, tt.failed, "expected BuildBinary to call Fatalf for unknown binary")
}

// subTestRecorder is a minimal *testing.T-shaped helper that captures
// Fatalf calls without aborting the parent test.
type subTestRecorder struct {
	*testing.T
	failed bool
}

func (s *subTestRecorder) Fatalf(format string, args ...any) {
	s.failed = true
	panic("fatalf-called")
}
