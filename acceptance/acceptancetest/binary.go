//go:build acceptance

// Package acceptancetest is a subprocess-based test harness for gitlab-shell.
//
// Tests under the acceptance build tag use Run to spawn a real binary from
// cmd/ against fake upstreams (typically built with github.com/elliotforbes/fakes)
// and assert on exit code, stdout, and stderr.
package acceptancetest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
)

// binaryCache memoises one path per binary name for the lifetime of the
// test process. The first call to BuildBinary for a name compiles the
// binary; later calls return the cached path. We keep one entry per name
// (rather than per test) because every test in the suite uses the same
// binaries — recompiling per-test would multiply CI time without
// catching anything that the once-per-process build doesn't.
var binaryCache sync.Map // name -> *binaryEntry

type binaryEntry struct {
	once sync.Once
	path string
	err  error
}

// BuildBinary compiles ./cmd/<name> to a temp dir on first call per name,
// then returns the cached path on subsequent calls. The binary cache
// directory lives at <repo>/tmp/acceptance-binaries and is gitignored.
//
// If the binary cannot be built, BuildBinary calls t.Fatalf — a build
// failure isn't behaviour of the binary under test.
func BuildBinary(t testingT, name string) string {
	t.Helper()

	entry, _ := binaryCache.LoadOrStore(name, &binaryEntry{})
	be := entry.(*binaryEntry)

	be.once.Do(func() {
		repoRoot, err := findRepoRoot()
		if err != nil {
			be.err = fmt.Errorf("finding repo root: %w", err)
			return
		}

		dir := filepath.Join(repoRoot, "tmp", "acceptance-binaries")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			be.err = fmt.Errorf("creating binary cache dir: %w", err)
			return
		}

		out := filepath.Join(dir, name)
		cmd := exec.Command("go", "build", "-o", out, "./cmd/"+name)
		cmd.Dir = repoRoot
		if buildOutput, err := cmd.CombinedOutput(); err != nil {
			be.err = fmt.Errorf("go build %s: %w\n%s", name, err, buildOutput)
			return
		}
		be.path = out
	})

	if be.err != nil {
		t.Fatalf("BuildBinary(%q): %v", name, be.err)
	}
	return be.path
}

// testingT is the subset of *testing.T used by BuildBinary. It exists so
// the harness's own tests can substitute a recorder that captures Fatalf
// without aborting the parent test.
type testingT interface {
	Helper()
	Fatalf(format string, args ...any)
}

// findRepoRoot walks up from this source file until it finds go.mod.
func findRepoRoot() (string, error) {
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("runtime.Caller failed")
	}

	dir := filepath.Dir(here)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found from %s", here)
		}
		dir = parent
	}
}
