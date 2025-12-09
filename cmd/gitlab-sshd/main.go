// Package main implements the GitLab SSH daemon.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/sshd"

	"gitlab.com/gitlab-org/labkit/fields"
	"gitlab.com/gitlab-org/labkit/monitoring"
	"gitlab.com/gitlab-org/labkit/v2/log"
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
	logger := log.New()
	command.CheckForVersionFlag(os.Args, Version, BuildTime)
	flag.Parse()

	cfg := new(config.Config)
	if *configDir != "" {
		var err error
		cfg, err = config.NewFromDir(*configDir)
		if err != nil {
			logger.ErrorContext(
				ctx,
				"failed to load configuration from specified directory",
				slog.String(fields.ErrorMessage, err.Error()),
			)
			return
		}
	}

	overrideConfigFromEnvironment(cfg)
	if err := cfg.IsSane(); err != nil {
		ctx = log.WithFields(context.Background(), slog.String(
			fields.ErrorMessage, err.Error(),
		))
		if *configDir == "" {
			logger.ErrorContext(ctx, "no config-dir provided, using only environment variables")
		} else {
			logger.ErrorContext(ctx, "configuration error")
		}
		return
	}

	cfg.ApplyGlobalState()

	ctx, finished := command.Setup("gitlab-sshd", cfg)
	defer finished()

	cfg.GitalyClient.InitSidechannelRegistry(ctx)

	server, err := sshd.NewServer(cfg)
	if err != nil {
		logger.ErrorContext(ctx, "Failed to start GitLab built-in sshd",
			slog.String(fields.ErrorMessage, err.Error()),
		)
		return
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
		logger.ErrorContext(ctx, "GitLab built-in sshd failed to listen for new connections",
			slog.String(fields.ErrorMessage, err.Error()))
		return
	}
}

func gracefulShutdown(
	ctx context.Context,
	done chan os.Signal,
	cfg *config.Config,
	server *sshd.Server,
	cancel context.CancelFunc,
) {
	go func() {
		sig := <-done
		signal.Reset(syscall.SIGINT, syscall.SIGTERM)
		logger := log.New()

		gracePeriod := time.Duration(cfg.Server.GracePeriod)
		logger.InfoContext(ctx, "Shutdown initiated",
			slog.Float64("shutdown_timeout_s", gracePeriod.Seconds()),
			slog.String("signal", sig.String()),
		)

		if err := server.Shutdown(); err != nil {
			logger.ErrorContext(ctx, "Error shutting down the server", slog.String(fields.ErrorMessage, err.Error()))
			return
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
		logger := log.New()
		logger.Error("monitoring service raised an error", slog.String(
			fields.ErrorMessage, err.Error(),
		))
	}()
}
