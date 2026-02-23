// Package metrics provides Prometheus metrics for monitoring gitlab-shell components.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	namespace       = "gitlab_shell"
	sshdSubsystem   = "sshd"
	httpSubsystem   = "http"
	gitalySubsystem = "gitaly"

	httpInFlightRequestsMetricName       = "in_flight_requests"
	httpRequestsTotalMetricName          = "requests_total"
	httpRequestDurationSecondsMetricName = "request_duration_seconds"

	sshdConnectionsInFlightName               = "in_flight_connections"
	sshdHitMaxSessionsName                    = "concurrent_limited_sessions_total"
	sshdSessionDurationSecondsName            = "session_duration_seconds"
	sshdSessionEstablishedDurationSecondsName = "session_established_duration_seconds"

	sliSshdSessionsTotalName       = "gitlab_sli:shell_sshd_sessions:total"
	sliSshdSessionsErrorsTotalName = "gitlab_sli:shell_sshd_sessions:errors_total"

	lfsHTTPConnectionsTotalName = "lfs_http_connections_total"
	lfsSSHConnectionsTotalName  = "lfs_ssh_connections_total"

	gitalyConnectionsTotalName = "connections_total"
)

var (
	// SshdSessionDuration is a histogram of latencies for connections to gitlab-shell sshd.
	SshdSessionDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: sshdSubsystem,
			Name:      sshdSessionDurationSecondsName,
			Help:      "A histogram of latencies for connections to gitlab-shell sshd.",
			Buckets: []float64{
				5.0,  /* 5s */
				30.0, /* 30s */
				60.0, /* 1m */
			},
		},
	)

	// SshdSessionEstablishedDuration is a histogram of latencies until session established to gitlab-shell sshd.
	SshdSessionEstablishedDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: sshdSubsystem,
			Name:      sshdSessionEstablishedDurationSecondsName,
			Help:      "A histogram of latencies until session established to gitlab-shell sshd.",
			Buckets: []float64{
				0.5, /* 5ms */
				1.0, /* 1s */
				5.0, /* 5s */
			},
		},
	)

	// SshdConnectionsInFlight is a gauge of connections currently being served by gitlab-shell sshd.
	SshdConnectionsInFlight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: sshdSubsystem,
			Name:      sshdConnectionsInFlightName,
			Help:      "A gauge of connections currently being served by gitlab-shell sshd.",
		},
	)

	// SshdHitMaxSessions is the number of times the concurrent sessions limit was hit in gitlab-shell sshd.
	SshdHitMaxSessions = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: sshdSubsystem,
			Name:      sshdHitMaxSessionsName,
			Help:      "The number of times the concurrent sessions limit was hit in gitlab-shell sshd.",
		},
	)

	// SliSshdSessionsTotal is the number of SSH sessions that have been established.
	SliSshdSessionsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: sliSshdSessionsTotalName,
			Help: "Number of SSH sessions that have been established",
		},
	)

	// SliSshdSessionsErrorsTotal is the number of SSH sessions that have failed.
	SliSshdSessionsErrorsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: sliSshdSessionsErrorsTotalName,
			Help: "Number of SSH sessions that have failed",
		},
	)

	// GitalyConnectionsTotal is a counter for the number of Gitaly connections that have been established,
	GitalyConnectionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: gitalySubsystem,
			Name:      gitalyConnectionsTotalName,
			Help:      "Number of Gitaly connections that have been established",
		},
		[]string{"status"},
	)

	// The metrics and the buckets size are similar to the ones we have for handlers in Labkit
	// When the MR: https://gitlab.com/gitlab-org/labkit/-/merge_requests/150 is merged,
	// these metrics can be refactored out of Gitlab Shell code by using the helper function from Labkit
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: httpSubsystem,
			Name:      httpRequestsTotalMetricName,
			Help:      "A counter for http requests.",
		},
		[]string{"code", "method"},
	)

	httpRequestDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: httpSubsystem,
			Name:      httpRequestDurationSecondsMetricName,
			Help:      "A histogram of latencies for http requests.",
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
		[]string{"code", "method"},
	)

	httpInFlightRequests = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: httpSubsystem,
			Name:      httpInFlightRequestsMetricName,
			Help:      "A gauge of requests currently being performed.",
		},
	)

	// LfsHTTPConnectionsTotal is the number of LFS over HTTP connections that have been established.
	LfsHTTPConnectionsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: lfsHTTPConnectionsTotalName,
			Help: "Number of LFS over HTTP connections that have been established",
		},
	)

	// LfsSSHConnectionsTotal is the number of LFS over SSH connections that have been established.
	LfsSSHConnectionsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: lfsSSHConnectionsTotalName,
			Help: "Number of LFS over SSH connections that have been established",
		},
	)
)

// NewRoundTripper wraps an http.RoundTripper to instrument it with Prometheus metrics.
func NewRoundTripper(next http.RoundTripper) promhttp.RoundTripperFunc {
	rt := next

	rt = promhttp.InstrumentRoundTripperCounter(httpRequestsTotal, rt)
	rt = promhttp.InstrumentRoundTripperDuration(httpRequestDurationSeconds, rt)
	return promhttp.InstrumentRoundTripperInFlight(httpInFlightRequests, rt)
}
