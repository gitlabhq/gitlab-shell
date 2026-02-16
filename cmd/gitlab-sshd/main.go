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
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/logger"
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

// nolint
func main() {
	ctx := context.Background()
	command.CheckForVersionFlag(os.Args, Version, BuildTime)
	flag.Parse()

	cfg := new(config.Config)
	if *configDir != "" {
		var err error
		cfg, err = config.NewFromDir(*configDir)
		if err != nil {
			v2log.New().ErrorContext(ctx, "failed to load configuration from specified directory", slog.String(
				fields.ErrorMessage, err.Error(),
			))
		}
	}
	logCloser := logger.ConfigureLogger(cfg)
	if logCloser != nil {
		defer logCloser.Close() //nolint:errcheck
	}
	slog.InfoContext(ctx, "gitlab-sshd starting up...")

	overrideConfigFromEnvironment(cfg)
	if err := isConfigSane(cfg); err != nil {
		ctx = v2log.WithFields(ctx, slog.String(fields.ErrorMessage, err.Error()))
		if *configDir == "" {
			slog.ErrorContext(ctx, "no config-dir provided, using only environment variables")
		} else {
			slog.ErrorContext(ctx, "configuration error")
		}
	}

	cfg.ApplyGlobalState()
	ctx, finished := command.Setup("gitlab-sshd", cfg)
	defer finished()

	cfg.GitalyClient.InitSidechannelRegistry(ctx)

	server, err := sshd.NewServer(cfg)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to start Gitlab built-in sshd", slog.String(
			fields.ErrorMessage, err.Error(),
		))
	}

	// Startup monitoring endpoint.
	if cfg.Server.WebListen != "" {
		startupMonitoringEndpoint(ctx, cfg, server)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)

	gracefulShutdown(ctx, done, cfg, server, cancel)

	if err := server.ListenAndServe(ctx); err != nil {
		slog.ErrorContext(ctx, "GitLab built-in sshd failed to listen for new connections",
			slog.String(fields.ErrorMessage, err.Error()))
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

		gracePeriod := time.Duration(cfg.Server.GracePeriod)
		slog.InfoContext(ctx, fmt.Sprintf("Shutdown initiated with grace period: %f", gracePeriod.Seconds()),
			slog.String("signal", sig.String()))

		if err := server.Shutdown(); err != nil {
			slog.ErrorContext(ctx, "Error shutting down the server", slog.String(
				fields.ErrorMessage, err.Error()))
		}
		<-time.After(gracePeriod)

		cancel()
	}()
}

func startupMonitoringEndpoint(ctx context.Context, cfg *config.Config, server *sshd.Server) {
	go func() {
		err := monitoring.Start(
			monitoring.WithListenerAddress(cfg.Server.WebListen),
			monitoring.WithBuildInformation(Version, BuildTime),
			monitoring.WithServeMux(server.MonitoringServeMux()),
		)

		slog.ErrorContext(ctx, "monitoring service raised an error", slog.String(
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
