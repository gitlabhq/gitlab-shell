// Package main implements the GitLab SSH daemon.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/logger"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/sshd"

	"gitlab.com/gitlab-org/labkit/fields"
	"gitlab.com/gitlab-org/labkit/log"
	"gitlab.com/gitlab-org/labkit/monitoring"
	v2log "gitlab.com/gitlab-org/labkit/v2/log"
)

var (
	configDir = flag.String("config-dir", "", "The directory the config is in")

	// Version is the current version of gitlab-shell
	Version = "(unknown version)" // Set at build time in the Makefile
	// BuildTime signifies the time the binary was build
	BuildTime = "19700101.000000" // Set at build time in the Makefile
)

func overrideConfigFromEnvironment(cfg *config.Config) {
	if gitlabURL := os.Getenv("GITLAB_URL"); gitlabURL != "" {
		cfg.GitlabUrl = gitlabURL
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
}

func main() {
	ctx := context.Background()
	v2Logger := v2log.New()
	v2Logger.InfoContext(ctx, "gitlab-sshd starting up...")
	command.CheckForVersionFlag(os.Args, Version, BuildTime)

	flag.Parse()

	cfg := new(config.Config)
	if *configDir != "" {
		var err error
		cfg, err = config.NewFromDir(*configDir)
		if err != nil {
			v2Logger.ErrorContext(ctx, "failed to load configuration from specified directory", slog.String(
				fields.ErrorMessage, err.Error(),
			))
			log.WithError(err).Fatal("failed to load configuration from specified directory")
		}
	}

	overrideConfigFromEnvironment(cfg)
	if err := cfg.IsSane(); err != nil {
		ctx = v2log.WithFields(ctx,
			slog.String(fields.ErrorMessage, err.Error()),
		)
		if *configDir == "" {
			v2Logger.ErrorContext(ctx, "no config-dir provided, using only environment variables")
			log.WithError(err).Fatal("no config-dir provided, using only environment variables")
		} else {
			v2Logger.ErrorContext(ctx, "configuration error")
			log.WithError(err).Fatal("configuration error")
		}
	}

	cfg.ApplyGlobalState()

	logCloser := logger.ConfigureStandalone(cfg)
	defer func() {
		if err := logCloser.Close(); err != nil {
			log.WithError(err).Fatal("Error closing logCloser")
		}
	}()
	ctx, finished := command.Setup("gitlab-sshd", cfg)
	defer finished()

	cfg.GitalyClient.InitSidechannelRegistry(ctx)

	server, err := sshd.NewServer(cfg)
	if err != nil {
		log.WithError(err).Fatal("Failed to start GitLab built-in sshd")
	}

	// Startup monitoring endpoint.
	if cfg.Server.WebListen != "" {
		startupMonitoringEndpoint(cfg, server)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)

	gracefulShutdown(ctx, done, cfg, server, cancel)

	if err := server.ListenAndServe(ctx); err != nil {
		log.WithError(err).Fatal("GitLab built-in sshd failed to listen for new connections")
	}
}

func gracefulShutdown(ctx context.Context, done chan os.Signal, cfg *config.Config, server *sshd.Server, cancel context.CancelFunc) {
	go func() {
		v2Logger := v2log.New()
		sig := <-done
		signal.Reset(syscall.SIGINT, syscall.SIGTERM)

		gracePeriod := time.Duration(cfg.Server.GracePeriod)
		log.WithContextFields(ctx, log.Fields{"shutdown_timeout_s": gracePeriod.Seconds(), "signal": sig.String()}).Info("Shutdown initiated")
		v2Logger.InfoContext(ctx, fmt.Sprintf("v2log: Shutdown initiated with grace period: %d", gracePeriod.Seconds()),
			slog.String("signal", sig.String()))

		if err := server.Shutdown(); err != nil {
			log.WithError(err).Fatal("Error shutting down the server")
		}

		<-time.After(gracePeriod)

		cancel()
	}()
}

func startupMonitoringEndpoint(cfg *config.Config, server *sshd.Server) {
	go func() {
		err := monitoring.Start(
			monitoring.WithListenerAddress(cfg.Server.WebListen),
			monitoring.WithBuildInformation(Version, BuildTime),
			monitoring.WithServeMux(server.MonitoringServeMux()),
		)

		log.WithError(err).Fatal("monitoring service raised an error")
	}()
}
