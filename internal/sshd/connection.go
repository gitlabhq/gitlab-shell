package sshd

import (
	"net"
	"context"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/semaphore"

	"gitlab.com/gitlab-org/gitlab-shell/internal/metrics"

	"gitlab.com/gitlab-org/labkit/log"
)

type connection struct {
	concurrentSessions *semaphore.Weighted
	nconn net.Conn
	remoteAddr string
	started time.Time
}

type channelHandler func(context.Context, *ssh.ServerConn, ssh.Channel, <-chan *ssh.Request) error

func newConnection(maxSessions int64, nconn net.Conn) *connection {
	return &connection{
		concurrentSessions: semaphore.NewWeighted(maxSessions),
		nconn: nconn,
		remoteAddr: nconn.RemoteAddr().String(),
		started: time.Now(),
	}
}

func (c *connection) handle(ctx context.Context, cfg *ssh.ServerConfig, handler channelHandler) {
	ctxlog := log.WithContextFields(ctx, log.Fields{"remote_addr": c.remoteAddr})

	// Prevent a panic in a single connection from taking out the whole server
	defer func() {
		if err := recover(); err != nil {
			ctxlog.Warn("panic handling session")
		}

		metrics.SliSshdSessionsErrorsTotal.Inc()
	}()

	ctxlog.Info("server: handleConn: start")

	metrics.SshdConnectionsInFlight.Inc()
	defer func() {
		metrics.SshdConnectionsInFlight.Dec()
		metrics.SshdSessionDuration.Observe(time.Since(c.started).Seconds())
	}()

	// Initialize the connection with server
	sconn, chans, reqs, err := ssh.NewServerConn(c.nconn, cfg)

	// Track the time required to establish a session
	establishSessionDuration := time.Since(c.started).Seconds()
	metrics.SshdSessionEstablishedDuration.Observe(establishSessionDuration)

	// Most of the times a connection failes due to the client's misconfiguration or when
	// a client cancels a request, so we shouldn't treat them as an error
	// Warnings will helps us to track the errors whether they happend on the server side
	if err != nil {
		ctxlog.WithError(err).WithFields(log.Fields{
			"establish_session_duration_s": establishSessionDuration,
		}).Warn("conn: init: failed to initialize SSH connection")

		return
	}
	go ssh.DiscardRequests(reqs)

	// Handle incoming requests
	c.handleRequests(ctx, sconn, chans, handler)

	ctxlog.WithFields(log.Fields{
		"duration_s":                   time.Since(c.started).Seconds(),
		"establish_session_duration_s": establishSessionDuration,
	}).Info("server: handleConn: done")
}

func (c *connection) handleRequests(ctx context.Context, sconn *ssh.ServerConn, chans <-chan ssh.NewChannel, handler channelHandler) {
	ctxlog := log.WithContextFields(ctx, log.Fields{"remote_addr": c.remoteAddr})

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
			defer c.concurrentSessions.Release(1)

			// Prevent a panic in a single session from taking out the whole server
			defer func() {
				if err := recover(); err != nil {
					ctxlog.WithField("recovered_error", err).Warn("panic handling session")

					metrics.SliSshdSessionsErrorsTotal.Inc()
				}
			}()

			err := handler(ctx, sconn, channel, requests)
			if err != nil {
				metrics.SliSshdSessionsErrorsTotal.Inc()
			}

			ctxlog.Info("connection: handle: done")
		}()
	}
}
