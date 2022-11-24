package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/logger"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/sshd"

	"gitlab.com/gitlab-org/labkit/log"
	"gitlab.com/gitlab-org/labkit/monitoring"
)

var (
	configDir = flag.String("config-dir", "", "The directory the config is in")

	// BuildTime signifies the time the binary was build.
	BuildTime = "2021-02-16T09:28:07+01:00" // Set at build time in the Makefile
	// Version is the current version of GitLab Shell sshd.
	Version = "(unknown version)" // Set at build time in the Makefile
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
			log.WithError(err).Fatal("failed to load configuration from specified directory")
		}
	}
	overrideConfigFromEnvironment(cfg)
	if err := cfg.IsSane(); err != nil {
		if *configDir == "" {
			log.WithError(err).Fatal("no config-dir provided, using only environment variables")
		} else {
			log.WithError(err).Fatal("configuration error")
		}
	}

	cfg.ApplyGlobalState()

	logCloser := logger.ConfigureStandalone(cfg)
	defer logCloser.Close()

	ctx, finished := command.Setup("gitlab-sshd", cfg)
	defer finished()

	cfg.GitalyClient.InitSidechannelRegistry(ctx)

	sshd.LoadGSSAPILib(&cfg.Server.GSSAPI)

	server, err := sshd.NewServer(cfg)
	if err != nil {
		log.WithError(err).Fatal("Failed to start GitLab built-in sshd")
	}

	// Startup monitoring endpoint.
	if cfg.Server.WebListen != "" {
		go func() {
			err := monitoring.Start(
				monitoring.WithListenerAddress(cfg.Server.WebListen),
				monitoring.WithBuildInformation(Version, BuildTime),
				monitoring.WithServeMux(server.MonitoringServeMux()),
			)

			log.WithError(err).Fatal("monitoring service raised an error")
		}()
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-done
		signal.Reset(syscall.SIGINT, syscall.SIGTERM)

		gracePeriod := time.Duration(cfg.Server.GracePeriod)
		log.WithContextFields(ctx, log.Fields{"shutdown_timeout_s": gracePeriod.Seconds(), "signal": sig.String()}).Info("Shutdown initiated")

		server.Shutdown()

		<-time.After(gracePeriod)

		cancel()

	}()

	if err := server.ListenAndServe(ctx); err != nil {
		log.WithError(err).Fatal("GitLab built-in sshd failed to listen for new connections")
	}
}
