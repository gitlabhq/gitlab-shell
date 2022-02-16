package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	namespace     = "gitlab_shell"
	sshdSubsystem = "sshd"
	httpSubsystem = "http"

	httpInFlightRequestsMetricName       = "in_flight_requests"
	httpRequestsTotalMetricName          = "requests_total"
	httpRequestDurationSecondsMetricName = "request_duration_seconds"

	sshdConnectionsInFlightName = "in_flight_connections"
	sshdConnectionDuration      = "connection_duration_seconds"
	sshdHitMaxSessions          = "concurrent_limited_sessions_total"
)

var (
	SshdConnectionDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: sshdSubsystem,
			Name:      sshdConnectionDuration,
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
			Name:      sshdHitMaxSessions,
			Help:      "The number of times the concurrent sessions limit was hit in gitlab-shell sshd.",
		},
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
