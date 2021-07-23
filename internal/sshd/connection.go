package sshd

import (
	"context"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/semaphore"

	"gitlab.com/gitlab-org/gitlab-shell/internal/metrics"

	"gitlab.com/gitlab-org/labkit/log"
)

type connection struct {
	begin              time.Time
	concurrentSessions *semaphore.Weighted
	remoteAddr         string
}

type channelHandler func(context.Context, ssh.Channel, <-chan *ssh.Request)

func newConnection(maxSessions int64, remoteAddr string) *connection {
	return &connection{
		begin:              time.Now(),
		concurrentSessions: semaphore.NewWeighted(maxSessions),
		remoteAddr:         remoteAddr,
	}
}

func (c *connection) handle(ctx context.Context, chans <-chan ssh.NewChannel, handler channelHandler) {
	defer metrics.SshdConnectionDuration.Observe(time.Since(c.begin).Seconds())

	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}
		if !c.concurrentSessions.TryAcquire(1) {
			newChannel.Reject(ssh.ResourceShortage, "too many concurrent sessions")
			metrics.SshdHitMaxSessions.Inc()
			continue
		}
		channel, requests, err := newChannel.Accept()
		if err != nil {
			log.WithError(err).Info("could not accept channel")
			c.concurrentSessions.Release(1)
			continue
		}

		go func() {
			defer c.concurrentSessions.Release(1)

			// Prevent a panic in a single session from taking out the whole server
			defer func() {
				if err := recover(); err != nil {
					log.WithFields(log.Fields{"recovered_error": err}).Warnf("panic handling session from %s", c.remoteAddr)
				}
			}()

			handler(ctx, channel, requests)
		}()
	}
}
