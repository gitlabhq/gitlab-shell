// Package sshd provides functionality for SSH daemon connections.
package sshd

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/semaphore"
	grpccodes "google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/disallowedcommand"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/metrics"

	"gitlab.com/gitlab-org/labkit/log"
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
	log.WithContextFields(ctx, log.Fields{}).Info("server: handleConn: start")

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
	log.WithContextFields(ctx, log.Fields{"reason": reason}).Info("server: handleConn: done")
}

func (c *connection) initServerConn(ctx context.Context, srvCfg *ssh.ServerConfig) (*ssh.ServerConn, <-chan ssh.NewChannel, error) {
	if c.cfg.Server.LoginGraceTime > 0 {
		_ = c.nconn.SetDeadline(time.Now().Add(time.Duration(c.cfg.Server.LoginGraceTime)))
		defer func() { _ = c.nconn.SetDeadline(time.Time{}) }()
	}

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
			_ = newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}

		if !c.concurrentSessions.TryAcquire(1) {
			ctxlog.Info("connection: handleRequests: too many concurrent sessions")
			_ = newChannel.Reject(ssh.ResourceShortage, "too many concurrent sessions")
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
				duration := time.Since(started).Seconds()
				metrics.SshdSessionDuration.Observe(duration)
				ctxlog.WithFields(log.Fields{"duration_s": duration}).Info("connection: handleRequests: done")
			}(time.Now())

			defer c.concurrentSessions.Release(1)

			// Prevent a panic in a single session from taking out the whole server
			defer func() {
				if err := recover(); err != nil {
					ctxlog.WithField("recovered_error", err).Error("panic handling session")
				}
			}()

			metrics.SliSshdSessionsTotal.Inc()
			err := handler(ctx, sconn, channel, requests)
			if err != nil {
				c.trackError(ctxlog, err)
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
	ctxlog := log.WithContextFields(ctx, log.Fields{"remote_addr": c.remoteAddr})

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ctxlog.Debug("connection: sendKeepAliveMsg: send keepalive message to a client")

			status, result, err := sconn.SendRequest(KeepAliveMsg, true, nil)
			if err != nil {
				ctxlog.Errorf("Error occurred while sending request :%v", err)
				return
			}

			if status {
				ctxlog.Debugf("connection: sendKeepAliveMsg: response: %v", string(result))
			}
		}
	}
}

func (c *connection) trackError(ctxlog *logrus.Entry, err error) {
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
	ctxlog.WithError(err).Warn("connection: session error")
}
