package sshd

import (
	"context"
	"errors"
	"net"
	"strings"
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
	cfg                      *config.Config
	concurrentSessions       *semaphore.Weighted
	nconn                    net.Conn
	maxSessions              int64
	remoteAddr               string
	started                  time.Time
	establishSessionDuration float64
}

type channelHandler func(*ssh.ServerConn, ssh.Channel, <-chan *ssh.Request) error

func newConnection(cfg *config.Config, nconn net.Conn) *connection {
	maxSessions := cfg.Server.ConcurrentSessionsLimit

	return &connection{
		cfg:                cfg,
		maxSessions:        maxSessions,
		concurrentSessions: semaphore.NewWeighted(maxSessions),
		nconn:              nconn,
		remoteAddr:         nconn.RemoteAddr().String(),
		started:            time.Now(),
	}
}

func (c *connection) handle(ctx context.Context, srvCfg *ssh.ServerConfig, handler channelHandler) {
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
	log.WithContextFields(ctx, log.Fields{
		"duration_s":                   time.Since(c.started).Seconds(),
		"establish_session_duration_s": c.establishSessionDuration,
		"reason":                       reason,
	}).Info("server: handleConn: done")
}

func (c *connection) initServerConn(ctx context.Context, srvCfg *ssh.ServerConfig) (*ssh.ServerConn, <-chan ssh.NewChannel, error) {
	sconn, chans, reqs, err := ssh.NewServerConn(c.nconn, srvCfg)
	if err != nil {
		msg := "connection: initServerConn: failed to initialize SSH connection"

		logger := log.WithContextFields(ctx, log.Fields{"remote_addr": c.remoteAddr}).WithError(err)

		if strings.Contains(err.Error(), "no common algorithm for host key") || err.Error() == "EOF" {
			logger.Debug(msg)
		} else {
			logger.Warn(msg)
		}

		return nil, nil, err
	}
	go ssh.DiscardRequests(reqs)

	return sconn, chans, err
}

func (c *connection) handleRequests(ctx context.Context, sconn *ssh.ServerConn, chans <-chan ssh.NewChannel, handler channelHandler) {
	ctxlog := log.WithContextFields(ctx, log.Fields{"remote_addr": c.remoteAddr})

	for newChannel := range chans {
		ctxlog.WithField("channel_type", newChannel.ChannelType()).Info("connection: handle: new channel requested")
		if newChannel.ChannelType() != "session" {
			ctxlog.Info("connection: handleRequests: unknown channel type")
			newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}
		if !c.concurrentSessions.TryAcquire(1) {
			ctxlog.Info("connection: handleRequests: too many concurrent sessions")
			newChannel.Reject(ssh.ResourceShortage, "too many concurrent sessions")
			metrics.SshdHitMaxSessions.Inc()
			continue
		}
		channel, requests, err := newChannel.Accept()
		if err != nil {
			ctxlog.WithError(err).Error("connection: handleRequests: accepting channel failed")
			c.concurrentSessions.Release(1)
			continue
		}

		go func() {
			defer func(started time.Time) {
				metrics.SshdSessionDuration.Observe(time.Since(started).Seconds())
			}(time.Now())
			c.establishSessionDuration = time.Since(c.started).Seconds()

			defer c.concurrentSessions.Release(1)

			// Prevent a panic in a single session from taking out the whole server
			defer func() {
				if err := recover(); err != nil {
					ctxlog.WithField("recovered_error", err).Warn("panic handling session")
				}
			}()

			metrics.SliSshdSessionsTotal.Inc()
			err := handler(sconn, channel, requests)
			if err != nil {
				c.trackError(err)
			}

			ctxlog.Info("connection: handleRequests: done")
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

func (c *connection) sendKeepAliveMsg(ctx context.Context, sconn *ssh.ServerConn, ticker *time.Ticker) {
	ctxlog := log.WithContextFields(ctx, log.Fields{"remote_addr": c.remoteAddr})

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ctxlog.Debug("connection: sendKeepAliveMsg: send keepalive message to a client")

			sconn.SendRequest(KeepAliveMsg, true, nil)
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
