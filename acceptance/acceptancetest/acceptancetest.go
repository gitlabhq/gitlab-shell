//go:build acceptance

// Package acceptancetest is a subprocess-based test harness for gitlab-shell.
//
// Tests under the acceptance build tag use Run to spawn a real binary from
// cmd/ against fake upstreams (typically built with github.com/elliotforbes/fakes)
// and assert on exit code, stdout, and stderr.
package acceptancetest

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// Config describes everything the harness needs to launch a binary.
// Tests fill the fields they care about; unset fields use defaults
// suitable for "binary boots, hits the configured URLs once, exits".
type Config struct {
	// Binary names a directory under cmd/ — required.
	Binary string

	// InternalAPIURL is the gitlab_url written into config.yml.
	InternalAPIURL string

	// Secret is the JWT signing secret written into config.yml. The new
	// gitlab client refuses to construct itself with an empty secret, so
	// any healthcheck-touching test needs this set.
	Secret string

	// FeatureFlagURL, if non-empty, is set as FEATURE_FLAG_ENDPOINT.
	// When empty, the binary's feature-flag client is not initialised
	// and flag checks default to false (the "old client" path).
	FeatureFlagURL string

	// Args is appended to the binary's command line.
	Args []string

	// ExtraEnv is layered on top of the harness-managed environment.
	ExtraEnv map[string]string

	// Stdin, if non-nil, is piped to the binary's stdin.
	Stdin io.Reader

	// Timeout overrides the default 30s subprocess timeout.
	Timeout time.Duration
}

// Result is what Run returns: the exit code, captured stdout/stderr, and
// wall-clock duration of the spawned binary.
type Result struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
}

// Run builds cfg.Binary (cached per name), writes a config.yml into a
// per-test temp dir, and exec's the binary with GITLAB_SHELL_DIR pointing
// at that dir. Blocks until the binary exits or Timeout elapses.
//
// Spawning failures (build error, fork error) call t.Fatalf — they are
// not the binary's behaviour. The binary's own exit code, stdout, and
// stderr are returned in Result for the caller to assert on.
func Run(t *testing.T, cfg Config) Result {
	t.Helper()

	binaryPath := BuildBinary(t, cfg.Binary)

	configDir := t.TempDir()
	if err := writeConfigDir(configDir, configFields{
		GitlabURL: cfg.InternalAPIURL,
		Secret:    cfg.Secret,
	}); err != nil {
		t.Fatalf("acceptancetest.Run: write config: %v", err)
	}

	env := buildEnv(cfg, configDir)

	return runBinary(t, runBinaryArgs{
		Path:    binaryPath,
		Args:    cfg.Args,
		Env:     env,
		Stdin:   cfg.Stdin,
		Timeout: cfg.Timeout,
	})
}

// buildEnv produces the environment passed to the spawned binary.
// Starts from the parent's env minus a small denylist (keys the harness
// needs to control), then layers harness-managed values, then ExtraEnv.
func buildEnv(cfg Config, configDir string) []string {
	denylist := map[string]struct{}{
		"GITLAB_SHELL_DIR":      {},
		"FEATURE_FLAG_ENDPOINT": {},
	}
	for k := range cfg.ExtraEnv {
		denylist[k] = struct{}{}
	}

	var env []string
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if _, drop := denylist[key]; drop {
			continue
		}
		env = append(env, kv)
	}

	env = append(env, "GITLAB_SHELL_DIR="+configDir)
	if cfg.FeatureFlagURL != "" {
		env = append(env, "FEATURE_FLAG_ENDPOINT="+cfg.FeatureFlagURL)
	}
	for k, v := range cfg.ExtraEnv {
		env = append(env, k+"="+v)
	}
	return env
}
