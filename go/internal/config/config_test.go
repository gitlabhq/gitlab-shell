package config

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseConfig(t *testing.T) {
	testRoot := "/foo/bar"
	testCases := []struct {
		yaml      string
		path      string
		format    string
		gitlabUrl string
		migration MigrationConfig
	}{
		{path: "/foo/bar/gitlab-shell.log", format: "text"},
		{yaml: "log_file: my-log.log", path: "/foo/bar/my-log.log", format: "text"},
		{yaml: "log_file: /qux/my-log.log", path: "/qux/my-log.log", format: "text"},
		{yaml: "log_format: json", path: "/foo/bar/gitlab-shell.log", format: "json"},
		{
			yaml:      "migration:\n  enabled: true\n  features:\n    - foo\n    - bar",
			path:      "/foo/bar/gitlab-shell.log",
			format:    "text",
			migration: MigrationConfig{Enabled: true, Features: []string{"foo", "bar"}},
		},
		{
			yaml:      "gitlab_url: http+unix://%2Fpath%2Fto%2Fgitlab%2Fgitlab.socket",
			path:      "/foo/bar/gitlab-shell.log",
			format:    "text",
			gitlabUrl: "http+unix:///path/to/gitlab/gitlab.socket",
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("yaml input: %q", tc.yaml), func(t *testing.T) {
			cfg := Config{RootDir: testRoot}
			if err := parseConfig([]byte(tc.yaml), &cfg); err != nil {
				t.Fatal(err)
			}

			if cfg.Migration.Enabled != tc.migration.Enabled {
				t.Fatalf("migration.enabled: expected %v, got %v", tc.migration.Enabled, cfg.Migration.Enabled)
			}

			if strings.Join(cfg.Migration.Features, ":") != strings.Join(tc.migration.Features, ":") {
				t.Fatalf("migration.features: expected %#v, got %#v", tc.migration.Features, cfg.Migration.Features)
			}

			if cfg.LogFile != tc.path {
				t.Fatalf("expected %q, got %q", tc.path, cfg.LogFile)
			}

			if cfg.LogFormat != tc.format {
				t.Fatalf("expected %q, got %q", tc.format, cfg.LogFormat)
			}

			assert.Equal(t, tc.gitlabUrl, cfg.GitlabUrl)
		})
	}
}

func TestFeatureEnabled(t *testing.T) {
	testCases := []struct {
		desc          string
		config        *Config
		feature       string
		expectEnabled bool
	}{
		{
			desc: "When the protocol is supported and the feature enabled",
			config: &Config{
				GitlabUrl: "http+unix://gitlab.socket",
				Migration: MigrationConfig{Enabled: true, Features: []string{"discover"}},
			},
			feature:       "discover",
			expectEnabled: true,
		},
		{
			desc: "When the protocol is supported and the feature is not enabled",
			config: &Config{
				GitlabUrl: "http+unix://gitlab.socket",
				Migration: MigrationConfig{Enabled: true, Features: []string{}},
			},
			feature:       "discover",
			expectEnabled: false,
		},
		{
			desc: "When the protocol is supported and all features are disabled",
			config: &Config{
				GitlabUrl: "http+unix://gitlab.socket",
				Migration: MigrationConfig{Enabled: false, Features: []string{"discover"}},
			},
			feature:       "discover",
			expectEnabled: false,
		},
		{
			desc: "When the protocol is not supported",
			config: &Config{
				GitlabUrl: "https://localhost:3000",
				Migration: MigrationConfig{Enabled: true, Features: []string{"discover"}},
			},
			feature:       "discover",
			expectEnabled: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			assert.Equal(t, tc.expectEnabled, tc.config.FeatureEnabled(string(tc.feature)))
		})
	}
}
