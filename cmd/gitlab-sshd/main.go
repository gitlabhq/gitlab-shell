// Package main implements the GitLab SSH daemon.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
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
		cfg.GitlabURL = gitlabURL
	}
	if gitlabTracing := os.Getenv("GITLAB_TRACING"); gitlabTracing != "" {
		cfg.GitlabTracing = gitlabTracing
	}
	if gitlabShellSecret := os.Getenv("GITLAB_SHELL_SECRET"); gitlabShellSecret != "" {
		cfg.Secret = gitlabShellSecret
	}
}

func main() {
	ctx := context.Background()
	command.CheckForVersionFlag(os.Args, Version, BuildTime)

	flag.Parse()

	cfg := new(config.Config)
	if *configDir != "" {
		var err error
		cfg, err = config.NewFromDir(*configDir)
		if err != nil {
			v2log.New().ErrorContext(ctx, "v2log: failed to load configuration from specified directory", slog.String(
				fields.ErrorMessage, err.Error(),
			))
		}
	}
	log := v2log.New()
	log.InfoContext(ctx, "v2log: gitlab-sshd starting up...")

	overrideConfigFromEnvironment(cfg)
	if err := isConfigSane(cfg); err != nil {
		ctx = v2log.WithFields(ctx, slog.String(fields.ErrorMessage, err.Error()))
		if *configDir == "" {
			v2log.New().ErrorContext(ctx, "v2log: no config-dir provided, using only environment variables")
		} else {
			v2log.New().ErrorContext(ctx, "v2log: configuration error")
		}
	}

	cfg.ApplyGlobalState()
	ctx, finished := command.Setup("gitlab-sshd", cfg)
	defer finished()

	cfg.GitalyClient.InitSidechannelRegistry(ctx)

	server, err := sshd.NewServer(cfg)
	if err != nil {
		log.ErrorContext(ctx, "v2log: Failed to start Gitlab built-in sshd", slog.String(
			fields.ErrorMessage, err.Error(),
		))
	}

	// Startup monitoring endpoint.
	if cfg.Server.WebListen != "" {
		startupMonitoringEndpoint(ctx, cfg, server, log)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)

	gracefulShutdown(ctx, done, cfg, server, cancel, log)

	if err := server.ListenAndServe(ctx); err != nil {
		log.ErrorContext(ctx, "v2log: GitLab built-in sshd failed to listen for new connections",
			slog.String(fields.ErrorMessage, err.Error()))
	}
}

func gracefulShutdown(
	ctx context.Context,
	done chan os.Signal,
	cfg *config.Config,
	server *sshd.Server,
	cancel context.CancelFunc,
	log *slog.Logger,
) {
	go func() {
		sig := <-done
		signal.Reset(syscall.SIGINT, syscall.SIGTERM)

		gracePeriod := time.Duration(cfg.Server.GracePeriod)
		log.InfoContext(ctx, fmt.Sprintf("v2log: Shutdown initiated with grace period: %f", gracePeriod.Seconds()),
			slog.String("signal", sig.String()))

		if err := server.Shutdown(); err != nil {
			log.ErrorContext(ctx, "v2log: Error shutting down the server", slog.String(
				fields.ErrorMessage, err.Error()))
		}
		<-time.After(gracePeriod)

		cancel()
	}()
}

func startupMonitoringEndpoint(ctx context.Context, cfg *config.Config, server *sshd.Server, log *slog.Logger) {
	go func() {
		err := monitoring.Start(
			monitoring.WithListenerAddress(cfg.Server.WebListen),
			monitoring.WithBuildInformation(Version, BuildTime),
			monitoring.WithServeMux(server.MonitoringServeMux()),
		)

		log.ErrorContext(ctx, "monitoring service raised an error", slog.String(
			fields.ErrorMessage, err.Error(),
		))
		panic(err)
	}()
}

// isConfigSane checks if the given config fulfills the minimum requirements to be able to run.
// Any error returned by this function should be a startup error. On the other hand
// if this function returns nil, this doesn't guarantee the config will work, but it's
// at least worth a try.
func isConfigSane(cfg *config.Config) error {
	if cfg.GitlabURL == "" {
		return errors.New("gitlab_url is required")
	}
	if cfg.Secret == "" {
		return errors.New("secret or secret_file_path is required")
	}
	return nil
}
