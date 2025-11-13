// Package sshd provides functionality for SSH daemon connections.
package sshd

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/semaphore"
	grpccodes "google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/disallowedcommand"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/metrics"
)

const (
	// KeepAliveMsg is the message used for keeping SSH connections alive.
	KeepAliveMsg = "keepalive@openssh.com"

	// NotOurRefError represents the error message indicating that the git upload-pack is not our reference
	NotOurRefError = `exit status 128, stderr: "fatal: git upload-pack: not our ref `
)

// EOFTimeout specifies the timeout duration for EOF (End of File) in SSH connections
var EOFTimeout = 10 * time.Second

type connection struct {
	cfg                *config.Config
	concurrentSessions *semaphore.Weighted
	nconn              net.Conn
	maxSessions        int64
	remoteAddr         string
}

type channelHandler func(context.Context, *ssh.ServerConn, ssh.Channel, <-chan *ssh.Request) error

func newConnection(cfg *config.Config, nconn net.Conn) *connection {
	maxSessions := cfg.Server.ConcurrentSessionsLimit

	return &connection{
		cfg:                cfg,
		maxSessions:        maxSessions,
		concurrentSessions: semaphore.NewWeighted(maxSessions),
		nconn:              nconn,
		remoteAddr:         nconn.RemoteAddr().String(),
	}
}

func (c *connection) handle(ctx context.Context, srvCfg *ssh.ServerConfig, handler channelHandler) {
	slog.InfoContext(ctx, "server: handleConn: start")

	sconn, chans, err := c.initServerConn(ctx, srvCfg)
	if err != nil {
		return
	}

	if c.cfg.Server.ClientAliveInterval > 0 {
		ticker := time.NewTicker(time.Duration(c.cfg.Server.ClientAliveInterval))
		defer ticker.Stop()
		go c.sendKeepAliveMsg(ctx, sconn, ticker)
	}

	c.handleRequests(ctx, sconn, chans, handler)

	reason := sconn.Wait()
	slog.InfoContext(ctx, "server: handleConn: done", "reason", reason)
}

func (c *connection) initServerConn(ctx context.Context, srvCfg *ssh.ServerConfig) (*ssh.ServerConn, <-chan ssh.NewChannel, error) {
	if c.cfg.Server.LoginGraceTime > 0 {
		_ = c.nconn.SetDeadline(time.Now().Add(time.Duration(c.cfg.Server.LoginGraceTime)))
		defer func() { _ = c.nconn.SetDeadline(time.Time{}) }()
	}

	sconn, chans, reqs, err := ssh.NewServerConn(c.nconn, srvCfg)
	if err != nil {
		msg := "connection: initServerConn: failed to initialize SSH connection"

		if strings.Contains(err.Error(), "no common algorithm for host key") || err.Error() == "EOF" {
			slog.DebugContext(ctx, msg, "remote_addr", c.remoteAddr, "error", err)
		} else {
			slog.WarnContext(ctx, msg, "remote_addr", c.remoteAddr, "error", err)
		}

		return nil, nil, err
	}
	go ssh.DiscardRequests(reqs)

	return sconn, chans, err
}

func (c *connection) handleRequests(ctx context.Context, sconn *ssh.ServerConn, chans <-chan ssh.NewChannel, handler channelHandler) {
	for newChannel := range chans {
		slog.InfoContext(ctx, "connection: handle: new channel requested",
			"remote_addr", c.remoteAddr,
			"channel_type", newChannel.ChannelType())

		if newChannel.ChannelType() != "session" {
			slog.InfoContext(ctx, "connection: handleRequests: unknown channel type", "remote_addr", c.remoteAddr)
			_ = newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}

		if !c.concurrentSessions.TryAcquire(1) {
			slog.InfoContext(ctx, "connection: handleRequests: too many concurrent sessions", "remote_addr", c.remoteAddr)
			_ = newChannel.Reject(ssh.ResourceShortage, "too many concurrent sessions")
			metrics.SshdHitMaxSessions.Inc()
			continue
		}

		channel, requests, err := newChannel.Accept()
		if err != nil {
			slog.ErrorContext(ctx, "connection: handleRequests: accepting channel failed",
				"remote_addr", c.remoteAddr,
				"error", err)
			c.concurrentSessions.Release(1)
			continue
		}

		go func() {
			defer func(started time.Time) {
				duration := time.Since(started).Seconds()
				metrics.SshdSessionDuration.Observe(duration)
				slog.InfoContext(ctx, "connection: handleRequests: done",
					"remote_addr", c.remoteAddr,
					"duration_s", duration)
			}(time.Now())

			defer c.concurrentSessions.Release(1)

			// Prevent a panic in a single session from taking out the whole server
			defer func() {
				if err := recover(); err != nil {
					slog.ErrorContext(ctx, "panic handling session",
						"remote_addr", c.remoteAddr,
						"recovered_error", err)
				}
			}()

			metrics.SliSshdSessionsTotal.Inc()
			err := handler(ctx, sconn, channel, requests)
			if err != nil {
				c.trackError(ctx, err)
			}
		}()
	}

	// When a connection has been prematurely closed we block execution until all concurrent sessions are released
	// in order to allow Gitaly complete the operations and close all the channels gracefully.
	// If it didn't happen within timeout, we unblock the execution
	// Related issue: https://gitlab.com/gitlab-org/gitlab-shell/-/issues/563
	ctx, cancel := context.WithTimeout(ctx, EOFTimeout)
	defer cancel()
	_ = c.concurrentSessions.Acquire(ctx, c.maxSessions)
}

func (c *connection) sendKeepAliveMsg(ctx context.Context, sconn *ssh.ServerConn, ticker *time.Ticker) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			slog.DebugContext(ctx, "connection: sendKeepAliveMsg: send keepalive message to a client",
				"remote_addr", c.remoteAddr)

			status, payload, err := sconn.SendRequest(KeepAliveMsg, true, nil)
			if err != nil {
				slog.ErrorContext(ctx, "Error occurred while sending request",
					"remote_addr", c.remoteAddr,
					"error", err)
				return
			}

			if status {
				slog.DebugContext(ctx, "connection: sendKeepAliveMsg: payload",
					"remote_addr", c.remoteAddr,
					"payload", string(payload))
			}
		}
	}
}

func (c *connection) trackError(ctx context.Context, err error) {
	var apiError *client.APIError
	if errors.As(err, &apiError) {
		return
	}

	if errors.Is(err, disallowedcommand.Error) {
		return
	}

	grpcCode := grpcstatus.Code(err)
	if grpcCode == grpccodes.Canceled || grpcCode == grpccodes.Unavailable {
		return
	} else if grpcCode == grpccodes.Internal && strings.Contains(err.Error(), NotOurRefError) {
		return
	}

	metrics.SliSshdSessionsErrorsTotal.Inc()
	slog.WarnContext(ctx, "connection: session error",
		"remote_addr", c.remoteAddr,
		"error", err)
}
