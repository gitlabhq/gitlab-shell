package config

import (
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"path/filepath"

	yaml "gopkg.in/yaml.v2"
)

const (
	configFile            = "config.yml"
	logFile               = "gitlab-shell.log"
	defaultSecretFileName = ".gitlab_shell_secret"
)

type HttpSettingsConfig struct {
	User               string `yaml:"user"`
	Password           string `yaml:"password"`
	ReadTimeoutSeconds uint64 `yaml:"read_timeout"`
	CaFile             string `yaml:"ca_file"`
	CaPath             string `yaml:"ca_path"`
	SelfSignedCert     bool   `yaml:"self_signed_cert"`
}

type Config struct {
	RootDir        string
	LogFile        string             `yaml:"log_file"`
	LogFormat      string             `yaml:"log_format"`
	GitlabUrl      string             `yaml:"gitlab_url"`
	GitlabTracing  string             `yaml:"gitlab_tracing"`
	SecretFilePath string             `yaml:"secret_file"`
	Secret         string             `yaml:"secret"`
	HttpSettings   HttpSettingsConfig `yaml:"http_settings"`
	HttpClient     *HttpClient
}

func New() (*Config, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	return NewFromDir(dir)
}

func NewFromDir(dir string) (*Config, error) {
	return newFromFile(path.Join(dir, configFile))
}

func newFromFile(filename string) (*Config, error) {
	cfg := &Config{RootDir: path.Dir(filename)}

	configBytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	if err := parseConfig(configBytes, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// parseConfig expects YAML data in configBytes and a Config instance with RootDir set.
func parseConfig(configBytes []byte, cfg *Config) error {
	if err := yaml.Unmarshal(configBytes, cfg); err != nil {
		return err
	}

	if cfg.LogFile == "" {
		cfg.LogFile = logFile
	}

	if len(cfg.LogFile) > 0 && cfg.LogFile[0] != '/' {
		cfg.LogFile = path.Join(cfg.RootDir, cfg.LogFile)
	}

	if cfg.LogFormat == "" {
		cfg.LogFormat = "text"
	}

	if cfg.GitlabUrl != "" {
		unescapedUrl, err := url.PathUnescape(cfg.GitlabUrl)
		if err != nil {
			return err
		}

		cfg.GitlabUrl = unescapedUrl
	}

	if err := parseSecret(cfg); err != nil {
		return err
	}

	return nil
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

	secretFileContent, err := ioutil.ReadFile(cfg.SecretFilePath)
	if err != nil {
		return err
	}
	cfg.Secret = string(secretFileContent)

	return nil
}
