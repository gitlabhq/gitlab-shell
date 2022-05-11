package sshd

import (
	"context"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/semaphore"

	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/metrics"

	"gitlab.com/gitlab-org/labkit/log"
)

const KeepAliveMsg = "keepalive@openssh.com"

type connection struct {
	cfg                *config.Config
	concurrentSessions *semaphore.Weighted
	remoteAddr         string
	sconn              *ssh.ServerConn
}

type channelHandler func(context.Context, ssh.Channel, <-chan *ssh.Request)

func newConnection(cfg *config.Config, remoteAddr string, sconn *ssh.ServerConn) *connection {
	return &connection{
		cfg:                cfg,
		concurrentSessions: semaphore.NewWeighted(cfg.Server.ConcurrentSessionsLimit),
		remoteAddr:         remoteAddr,
		sconn:              sconn,
	}
}

func (c *connection) handle(ctx context.Context, chans <-chan ssh.NewChannel, handler channelHandler) {
	ctxlog := log.WithContextFields(ctx, log.Fields{"remote_addr": c.remoteAddr})

	if c.cfg.Server.ClientAliveIntervalSeconds > 0 {
		ticker := time.NewTicker(c.cfg.Server.ClientAliveInterval())
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

			handler(ctx, channel, requests)
			ctxlog.Info("connection: handle: done")
		}()
	}
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
