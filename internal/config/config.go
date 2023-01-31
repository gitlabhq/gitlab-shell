package config

import (
	"errors"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"

	yaml "gopkg.in/yaml.v2"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitaly"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/metrics"
)

const (
	configFile            = "config.yml"
	defaultSecretFileName = ".gitlab_shell_secret"
)

type YamlDuration time.Duration

type GSSAPIConfig struct {
	Enabled              bool   `yaml:"enabled,omitempty"`
	Keytab               string `yaml:"keytab,omitempty"`
	ServicePrincipalName string `yaml:"service_principal_name,omitempty"`
	LibPath              string
}

type ServerConfig struct {
	Listen                  string       `yaml:"listen,omitempty"`
	ProxyProtocol           bool         `yaml:"proxy_protocol,omitempty"`
	ProxyPolicy             string       `yaml:"proxy_policy,omitempty"`
	ProxyAllowed            []string     `yaml:"proxy_allowed,omitempty"`
	WebListen               string       `yaml:"web_listen,omitempty"`
	ConcurrentSessionsLimit int64        `yaml:"concurrent_sessions_limit,omitempty"`
	ClientAliveInterval     YamlDuration `yaml:"client_alive_interval,omitempty"`
	GracePeriod             YamlDuration `yaml:"grace_period"`
	ProxyHeaderTimeout      YamlDuration `yaml:"proxy_header_timeout"`
	LoginGraceTime          YamlDuration `yaml:"login_grace_time"`
	ReadinessProbe          string       `yaml:"readiness_probe"`
	LivenessProbe           string       `yaml:"liveness_probe"`
	HostKeyFiles            []string     `yaml:"host_key_files,omitempty"`
	HostCertFiles           []string     `yaml:"host_cert_files,omitempty"`
	MACs                    []string     `yaml:"macs"`
	KexAlgorithms           []string     `yaml:"kex_algorithms"`
	Ciphers                 []string     `yaml:"ciphers"`
	GSSAPI                  GSSAPIConfig `yaml:"gssapi,omitempty"`
}

type HttpSettingsConfig struct {
	User               string `yaml:"user"`
	Password           string `yaml:"password"`
	ReadTimeoutSeconds uint64 `yaml:"read_timeout"`
	CaFile             string `yaml:"ca_file"`
	CaPath             string `yaml:"ca_path"`
}

type Config struct {
	User                  string `yaml:"user,omitempty"`
	RootDir               string
	LogFile               string `yaml:"log_file,omitempty"`
	LogFormat             string `yaml:"log_format,omitempty"`
	LogLevel              string `yaml:"log_level,omitempty"`
	GitlabUrl             string `yaml:"gitlab_url"`
	GitlabRelativeURLRoot string `yaml:"gitlab_relative_url_root"`
	GitlabTracing         string `yaml:"gitlab_tracing"`
	// SecretFilePath is only for parsing. Application code should always use Secret.
	SecretFilePath string             `yaml:"secret_file"`
	Secret         string             `yaml:"secret"`
	SslCertDir     string             `yaml:"ssl_cert_dir"`
	HttpSettings   HttpSettingsConfig `yaml:"http_settings"`
	Server         ServerConfig       `yaml:"sshd"`

	httpClient     *client.HttpClient
	httpClientErr  error
	httpClientOnce sync.Once

	GitalyClient gitaly.Client
}

// The defaults to apply before parsing the config file(s).
var (
	DefaultConfig = Config{
		LogFile:   "gitlab-shell.log",
		LogFormat: "json",
		LogLevel:  "info",
		Server:    DefaultServerConfig,
		User:      "git",
	}

	DefaultServerConfig = ServerConfig{
		Listen:                  "[::]:22",
		WebListen:               "localhost:9122",
		ConcurrentSessionsLimit: 10,
		GracePeriod:             YamlDuration(10 * time.Second),
		ClientAliveInterval:     YamlDuration(15 * time.Second),
		ProxyHeaderTimeout:      YamlDuration(500 * time.Millisecond),
		LoginGraceTime:          YamlDuration(60 * time.Second),
		ReadinessProbe:          "/start",
		LivenessProbe:           "/health",
		HostKeyFiles: []string{
			"/run/secrets/ssh-hostkeys/ssh_host_rsa_key",
			"/run/secrets/ssh-hostkeys/ssh_host_ecdsa_key",
			"/run/secrets/ssh-hostkeys/ssh_host_ed25519_key",
		},
	}
)

func (d *YamlDuration) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var intDuration int
	if err := unmarshal(&intDuration); err != nil {
		return unmarshal((*time.Duration)(d))
	}

	*d = YamlDuration(time.Duration(intDuration) * time.Second)

	return nil
}

func (c *Config) ApplyGlobalState() {
	if c.SslCertDir != "" {
		os.Setenv("SSL_CERT_DIR", c.SslCertDir)
	}
}

func (c *Config) HttpClient() (*client.HttpClient, error) {
	c.httpClientOnce.Do(func() {
		client, err := client.NewHTTPClientWithOpts(
			c.GitlabUrl,
			c.GitlabRelativeURLRoot,
			c.HttpSettings.CaFile,
			c.HttpSettings.CaPath,
			c.HttpSettings.ReadTimeoutSeconds,
			nil,
		)
		if err != nil {
			c.httpClientErr = err
			return
		}

		tr := client.RetryableHTTP.HTTPClient.Transport
		client.RetryableHTTP.HTTPClient.Transport = metrics.NewRoundTripper(tr)

		c.httpClient = client
	})

	return c.httpClient, c.httpClientErr
}

// NewFromDirExternal returns a new config from a given root dir. It also applies defaults appropriate for
// gitlab-shell running in an external SSH server.
func NewFromDirExternal(dir string) (*Config, error) {
	cfg, err := newFromFile(filepath.Join(dir, configFile))
	if err != nil {
		return nil, err
	}

	cfg.ApplyGlobalState()

	return cfg, nil
}

// NewFromDir returns a new config given a root directory. It looks for the config file name in the
// given directory and reads the config from it. It doesn't apply any defaults. New code should prefer
// this over NewFromDirIntegrated and apply the right default via one of the Apply... functions.
func NewFromDir(dir string) (*Config, error) {
	return newFromFile(filepath.Join(dir, configFile))
}

// newFromFile reads a new Config instance from the given file path. It doesn't apply any defaults.
func newFromFile(path string) (*Config, error) {
	cfg := &Config{}
	*cfg = DefaultConfig
	cfg.RootDir = filepath.Dir(path)

	configBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if err := yaml.Unmarshal(configBytes, cfg); err != nil {
		return nil, err
	}

	if cfg.GitlabUrl != "" {
		// This is only done for historic reasons, don't implement it for new config sources.
		unescapedUrl, err := url.PathUnescape(cfg.GitlabUrl)
		if err != nil {
			return nil, err
		}

		cfg.GitlabUrl = unescapedUrl
	}

	if err := parseSecret(cfg); err != nil {
		return nil, err
	}

	if len(cfg.LogFile) > 0 && cfg.LogFile[0] != '/' && cfg.RootDir != "" {
		cfg.LogFile = filepath.Join(cfg.RootDir, cfg.LogFile)
	}

	return cfg, nil
}

func parseSecret(cfg *Config) error {
	// The secret was parsed from yaml no need to read another file
	if cfg.Secret != "" {
		return nil
	}

	if cfg.SecretFilePath == "" {
		cfg.SecretFilePath = defaultSecretFileName
	}

	if !filepath.IsAbs(cfg.SecretFilePath) {
		cfg.SecretFilePath = path.Join(cfg.RootDir, cfg.SecretFilePath)
	}

	secretFileContent, err := os.ReadFile(cfg.SecretFilePath)
	if err != nil {
		return err
	}
	cfg.Secret = string(secretFileContent)

	return nil
}

// IsSane checks if the given config fulfills the minimum requirements to be able to run.
// Any error returned by this function should be a startup error. On the other hand
// if this function returns nil, this doesn't guarantee the config will work, but it's
// at least worth a try.
func (cfg *Config) IsSane() error {
	if cfg.GitlabUrl == "" {
		return errors.New("gitlab_url is required")
	}
	if cfg.Secret == "" {
		return errors.New("secret or secret_file_path is required")
	}
	return nil
}
