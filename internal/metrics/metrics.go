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
	sshdCanceledSessionsName                  = "canceled_sessions"

	sliSshdSessionsTotalName       = "gitlab_sli:shell_sshd_sessions:total"
	sliSshdSessionsErrorsTotalName = "gitlab_sli:shell_sshd_sessions:errors_total"

	gitalyConnectionsTotalName = "connections_total"
)

var (
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

	SshdConnectionsInFlight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: sshdSubsystem,
			Name:      sshdConnectionsInFlightName,
			Help:      "A gauge of connections currently being served by gitlab-shell sshd.",
		},
	)

	SshdHitMaxSessions = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: sshdSubsystem,
			Name:      sshdHitMaxSessionsName,
			Help:      "The number of times the concurrent sessions limit was hit in gitlab-shell sshd.",
		},
	)

	SliSshdSessionsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: sliSshdSessionsTotalName,
			Help: "Number of SSH sessions that have been established",
		},
	)

	SliSshdSessionsErrorsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: sliSshdSessionsErrorsTotalName,
			Help: "Number of SSH sessions that have failed",
		},
	)

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
)

func NewRoundTripper(next http.RoundTripper) promhttp.RoundTripperFunc {
	rt := next

	rt = promhttp.InstrumentRoundTripperCounter(httpRequestsTotal, rt)
	rt = promhttp.InstrumentRoundTripperDuration(httpRequestDurationSeconds, rt)
	return promhttp.InstrumentRoundTripperInFlight(httpInFlightRequests, rt)
}
