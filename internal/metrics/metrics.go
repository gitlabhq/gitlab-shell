package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	namespace     = "gitlab_shell"
	sshdSubsystem = "sshd"
	httpSubsystem = "http"
)

var (
	SshdConnectionDuration = promauto.NewHistogram(
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

	SshdHitMaxSessions = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: sshdSubsystem,
			Name:      "concurrent_limited_sessions_total",
			Help:      "The number of times the concurrent sessions limit was hit in gitlab-shell sshd.",
		},
	)

	HttpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: httpSubsystem,
			Name:      "request_seconds",
			Help:      "A histogram of latencies for gitlab-shell http requests",
			Buckets: []float64{
				0.01, /* 10ms */
				0.05, /* 50ms */
				0.1,  /* 100ms */
				0.25, /* 250ms */
				0.5,  /* 500ms */
				1.0,  /* 1s */
				5.0,  /* 5s */
				10.0, /* 10s */
			},
		},
		[]string{"code", "method"},
	)
)
