package sshd

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/semaphore"
)

const (
	namespace     = "gitlab_shell"
	sshdSubsystem = "sshd"
)

var (
	sshdConnectionDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: sshdSubsystem,
			Name:      "connection_duration_seconds",
			Help:      "A histogram of latencies for connections to gitlab-shell sshd.",
			Buckets: []float64{
				0.005, /* 5ms */
				0.025, /* 25ms */
				0.1,   /* 100ms */
				0.5,   /* 500ms */
				1.0,   /* 1s */
				10.0,  /* 10s */
				30.0,  /* 30s */
				60.0,  /* 1m */
				300.0, /* 5m */
			},
		},
	)

	sshdHitMaxSessions = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: sshdSubsystem,
			Name:      "concurrent_limited_sessions_total",
			Help:      "The number of times the concurrent sessions limit was hit in gitlab-shell sshd.",
		},
	)
)

type connection struct {
	begin              time.Time
	concurrentSessions *semaphore.Weighted
}

type channelHandler func(context.Context, ssh.Channel, <-chan *ssh.Request)

func newConnection(maxSessions int64) *connection {
	return &connection{
		begin:              time.Now(),
		concurrentSessions: semaphore.NewWeighted(maxSessions),
	}
}

func (c *connection) handle(ctx context.Context, chans <-chan ssh.NewChannel, handler channelHandler) {
	defer sshdConnectionDuration.Observe(time.Since(c.begin).Seconds())

	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}
		if !c.concurrentSessions.TryAcquire(1) {
			newChannel.Reject(ssh.ResourceShortage, "too many concurrent sessions")
			sshdHitMaxSessions.Inc()
			continue
		}
		channel, requests, err := newChannel.Accept()
		if err != nil {
			log.Infof("Could not accept channel: %v", err)
			c.concurrentSessions.Release(1)
			continue
		}

		go func() {
			defer c.concurrentSessions.Release(1)
			handler(ctx, channel, requests)
		}()
	}
}
