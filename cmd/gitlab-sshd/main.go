package main

import (
	"flag"
	"os"

	log "github.com/sirupsen/logrus"

	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/logger"
	"gitlab.com/gitlab-org/gitlab-shell/internal/sshd"
)

var (
	configDir = flag.String("config-dir", "", "The directory the config is in")
)

func overrideConfigFromEnvironment(cfg *config.Config) {
	if gitlabUrl := os.Getenv("GITLAB_URL"); gitlabUrl != "" {
		cfg.GitlabUrl = gitlabUrl
	}
	if gitlabTracing := os.Getenv("GITLAB_TRACING"); gitlabTracing != "" {
		cfg.GitlabTracing = gitlabTracing
	}
	if gitlabShellSecret := os.Getenv("GITLAB_SHELL_SECRET"); gitlabShellSecret != "" {
		cfg.Secret = gitlabShellSecret
	}
	if gitlabLogFormat := os.Getenv("GITLAB_LOG_FORMAT"); gitlabLogFormat != "" {
		cfg.LogFormat = gitlabLogFormat
	}
	return
}

func main() {
	flag.Parse()
	cfg := new(config.Config)
	if *configDir != "" {
		var err error
		cfg, err = config.NewFromDir(*configDir)
		if err != nil {
			log.Fatalf("failed to load configuration from specified directory: %v", err)
		}
	}
	overrideConfigFromEnvironment(cfg)
	if err := cfg.IsSane(); err != nil {
		if *configDir == "" {
			log.Warn("note: no config-dir provided, using only environment variables")
		}
		log.Fatalf("configuration error: %v", err)
	}
	logger.ConfigureStandalone(cfg)

	if err := sshd.Run(cfg); err != nil {
		log.Fatalf("Failed to start GitLab built-in sshd: %v", err)
	}
}
