package client

import (
	"net/http"
	"time"

	"gitlab.com/gitlab-org/labkit/correlation"
	"gitlab.com/gitlab-org/labkit/log"
	"gitlab.com/gitlab-org/labkit/tracing"
)

type transport struct {
	next http.RoundTripper
}

func (rt *transport) RoundTrip(request *http.Request) (*http.Response, error) {
	ctx := request.Context()

	originalRemoteIP, ok := ctx.Value(OriginalRemoteIPContextKey{}).(string)
	if ok {
		request.Header.Add("X-Forwarded-For", originalRemoteIP)
	}
	request.Close = true
	request.Header.Add("User-Agent", defaultUserAgent)

	start := time.Now()

	response, err := rt.next.RoundTrip(request)

	fields := log.Fields{
		"method":      request.Method,
		"url":         request.URL.String(),
		"duration_ms": time.Since(start) / time.Millisecond,
	}
	logger := log.WithContextFields(ctx, fields)

	if err != nil {
		logger.WithError(err).Error("Internal API unreachable")
		return response, err
	}

	logger = logger.WithField("status", response.StatusCode)

	if response.StatusCode >= 400 {
		logger.WithError(err).Error("Internal API error")
		return response, err
	}

	if response.ContentLength >= 0 {
		logger = logger.WithField("content_length_bytes", response.ContentLength)
	}

	logger.Info("Finished HTTP request")

	return response, nil
}

func newTransport(next http.RoundTripper) http.RoundTripper {
	t := &transport{next: next}
	return correlation.NewInstrumentedRoundTripper(tracing.NewRoundTripper(t))
}
