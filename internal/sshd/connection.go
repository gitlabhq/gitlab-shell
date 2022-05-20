package sshd

import (
	"context"
	"errors"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/semaphore"
	grpccodes "google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	"gitlab.com/gitlab-org/gitlab-shell/client"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/metrics"

	"gitlab.com/gitlab-org/labkit/log"
)

const KeepAliveMsg = "keepalive@openssh.com"

var EOFTimeout = 10 * time.Second

type connection struct {
	cfg                *config.Config
	concurrentSessions *semaphore.Weighted
	remoteAddr         string
	sconn              *ssh.ServerConn
	maxSessions        int64
}

type channelHandler func(context.Context, ssh.Channel, <-chan *ssh.Request) error

func newConnection(cfg *config.Config, remoteAddr string, sconn *ssh.ServerConn) *connection {
	maxSessions := cfg.Server.ConcurrentSessionsLimit

	return &connection{
		cfg:                cfg,
		maxSessions:        maxSessions,
		concurrentSessions: semaphore.NewWeighted(maxSessions),
		remoteAddr:         remoteAddr,
		sconn:              sconn,
	}
}

func (c *connection) handle(ctx context.Context, chans <-chan ssh.NewChannel, handler channelHandler) {
	ctxlog := log.WithContextFields(ctx, log.Fields{"remote_addr": c.remoteAddr})

	if c.cfg.Server.ClientAliveInterval > 0 {
		ticker := time.NewTicker(time.Duration(c.cfg.Server.ClientAliveInterval))
		defer ticker.Stop()
		go c.sendKeepAliveMsg(ctx, ticker)
	}

	for newChannel := range chans {
		ctxlog.WithField("channel_type", newChannel.ChannelType()).Info("connection: handle: new channel requested")
		if newChannel.ChannelType() != "session" {
			ctxlog.Info("connection: handle: unknown channel type")
			newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}
		if !c.concurrentSessions.TryAcquire(1) {
			ctxlog.Info("connection: handle: too many concurrent sessions")
			newChannel.Reject(ssh.ResourceShortage, "too many concurrent sessions")
			metrics.SshdHitMaxSessions.Inc()
			continue
		}
		channel, requests, err := newChannel.Accept()
		if err != nil {
			ctxlog.WithError(err).Error("connection: handle: accepting channel failed")
			c.concurrentSessions.Release(1)
			continue
		}

		go func() {
			defer func(started time.Time) {
				metrics.SshdSessionDuration.Observe(time.Since(started).Seconds())
			}(time.Now())

			defer c.concurrentSessions.Release(1)

			// Prevent a panic in a single session from taking out the whole server
			defer func() {
				if err := recover(); err != nil {
					ctxlog.WithField("recovered_error", err).Warn("panic handling session")
				}
			}()

			metrics.SliSshdSessionsTotal.Inc()
			err := handler(ctx, channel, requests)
			if err != nil {
				c.trackError(err)
			}

			ctxlog.Info("connection: handle: done")
		}()
	}

	// When a connection has been prematurely closed we block execution until all concurrent sessions are released
	// in order to allow Gitaly complete the operations and close all the channels gracefully.
	// If it didn't happen within timeout, we unblock the execution
	// Related issue: https://gitlab.com/gitlab-org/gitlab-shell/-/issues/563
	ctx, cancel := context.WithTimeout(ctx, EOFTimeout)
	defer cancel()
	c.concurrentSessions.Acquire(ctx, c.maxSessions)
}

func (c *connection) sendKeepAliveMsg(ctx context.Context, ticker *time.Ticker) {
	ctxlog := log.WithContextFields(ctx, log.Fields{"remote_addr": c.remoteAddr})

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ctxlog.Debug("session: handleShell: send keepalive message to a client")

			c.sconn.SendRequest(KeepAliveMsg, true, nil)
		}
	}
}

func (c *connection) trackError(err error) {
	var apiError *client.ApiError
	if errors.As(err, &apiError) {
		return
	}

	grpcCode := grpcstatus.Code(err)
	if grpcCode == grpccodes.Canceled || grpcCode == grpccodes.Unavailable {
		return
	}

	metrics.SliSshdSessionsErrorsTotal.Inc()
}
