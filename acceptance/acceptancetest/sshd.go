//go:build acceptance

package acceptancetest

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
	"gopkg.in/yaml.v3"
)

// SSHDConfig configures a gitlab-sshd daemon for acceptance tests.
type SSHDConfig struct {
	// InternalAPIURL is written as gitlab_url.
	InternalAPIURL string
	// Secret is the JWT signing secret.
	Secret string
	// User is the SSH login user; defaults to "git".
	User string
	// FeatureFlagURL, if non-empty, is exported as FEATURE_FLAG_ENDPOINT.
	FeatureFlagURL string
	// ExtraEnv is layered on top of the daemon's environment.
	ExtraEnv map[string]string
}

// SSHD is a handle to a running gitlab-sshd daemon.
type SSHD struct {
	// Addr is the host:port the daemon is listening on.
	Addr string
}

// sshdYAML is the minimal config.yml gitlab-sshd needs to boot for cert auth.
type sshdYAML struct {
	GitlabURL string `yaml:"gitlab_url"`
	Secret    string `yaml:"secret"`
	User      string `yaml:"user"`
	SSHD      struct {
		Listen       string   `yaml:"listen"`
		WebListen    string   `yaml:"web_listen"`
		HostKeyFiles []string `yaml:"host_key_files"`
	} `yaml:"sshd"`
}

// StartSSHD boots a gitlab-sshd daemon bound to an ephemeral loopback port,
// waits until it is listening, and registers teardown via t.Cleanup.
func StartSSHD(t *testing.T, cfg SSHDConfig) *SSHD {
	t.Helper()

	user := cfg.User
	if user == "" {
		user = "git"
	}

	dir := t.TempDir()
	hostKeyPath := writeHostKey(t, dir)
	addr := freePort(t)

	var conf sshdYAML
	conf.GitlabURL = cfg.InternalAPIURL
	conf.Secret = cfg.Secret
	conf.User = user
	conf.SSHD.Listen = addr
	conf.SSHD.WebListen = "" // disable the monitoring endpoint
	conf.SSHD.HostKeyFiles = []string{hostKeyPath}

	out, err := yaml.Marshal(conf)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yml"), out, 0o600))

	binaryPath := BuildBinary(t, "gitlab-sshd")

	cmd := exec.Command(binaryPath, "-config-dir", dir)
	cmd.Env = sshdEnv(cfg)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stderr

	require.NoError(t, cmd.Start())

	t.Cleanup(func() {
		_ = cmd.Process.Signal(os.Interrupt)
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			_ = cmd.Process.Kill()
			<-done // ensure the Wait goroutine completes and the process is reaped
		}
		if t.Failed() {
			t.Logf("gitlab-sshd output:\n%s", stderr.String())
		}
	})

	waitForTCP(t, addr, 5*time.Second, stderr.String)

	return &SSHD{Addr: addr}
}

// sshdEnv builds the daemon environment: the parent env minus the keys the
// harness controls, plus harness-managed values and ExtraEnv.
func sshdEnv(cfg SSHDConfig) []string {
	denylist := map[string]struct{}{"FEATURE_FLAG_ENDPOINT": {}}
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
	if cfg.FeatureFlagURL != "" {
		env = append(env, "FEATURE_FLAG_ENDPOINT="+cfg.FeatureFlagURL)
	}
	for k, v := range cfg.ExtraEnv {
		env = append(env, k+"="+v)
	}
	return env
}

// writeHostKey generates an ed25519 host key, writes it as an OpenSSH PEM under
// dir, and returns the file path.
func writeHostKey(t *testing.T, dir string) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	block, err := ssh.MarshalPrivateKey(priv, "")
	require.NoError(t, err)
	path := filepath.Join(dir, "host_key")
	require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(block), 0o600))
	return path
}

// freePort reserves a loopback port by binding and immediately releasing it,
// returning the host:port string. There is a small race window before the
// daemon re-binds; acceptable for tests.
func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())
	return addr
}

// waitForTCP polls addr until a TCP connection succeeds or timeout elapses.
func waitForTCP(t *testing.T, addr string, timeout time.Duration, diag func() string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("gitlab-sshd did not listen on %s within %s\noutput:\n%s", addr, timeout, diag())
}
