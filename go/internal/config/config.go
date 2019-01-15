package config

import (
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"strings"

	yaml "gopkg.in/yaml.v2"
)

const (
	configFile = "config.yml"
	logFile    = "gitlab-shell.log"
)

type MigrationConfig struct {
	Enabled  bool     `yaml:"enabled"`
	Features []string `yaml:"features"`
}

type Config struct {
	RootDir   string
	LogFile   string          `yaml:"log_file"`
	LogFormat string          `yaml:"log_format"`
	Migration MigrationConfig `yaml:"migration"`
	GitlabUrl string          `yaml:"gitlab_url"`
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

func (c *Config) FeatureEnabled(featureName string) bool {
	if !c.Migration.Enabled {
		return false
	}

	if !strings.HasPrefix(c.GitlabUrl, "http+unix://") {
		return false
	}

	for _, enabledFeature := range c.Migration.Features {
		if enabledFeature == featureName {
			return true
		}
	}

	return false
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

	return nil
}
