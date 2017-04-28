package config

import (
	"testing"
)

func TestConfigLogFile(t *testing.T) {
	testRoot := "/foo/bar"
	testCases := []struct {
		yaml string
		path string
	}{
		{path: "/foo/bar/gitlab-shell.log"},
		{yaml: "log_file: my-log.log", path: "/foo/bar/my-log.log"},
		{yaml: "log_file: /qux/my-log.log", path: "/qux/my-log.log"},
	}

	for _, tc := range testCases {
		cfg := Config{RootDir: testRoot}
		if err := parseConfig([]byte(tc.yaml), &cfg); err != nil {
			t.Fatalf("%q: %v", tc.yaml, err)
		}

		if cfg.LogFile != tc.path {
			t.Fatalf("%q: expected %q, got %q", tc.yaml, tc.path, cfg.LogFile)
		}
	}
}
