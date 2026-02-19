package client

import (
	"net/http"
	"time"

	"gitlab.com/gitlab-org/labkit/correlation"
	"gitlab.com/gitlab-org/labkit/log"
	"gitlab.com/gitlab-org/labkit/tracing"
)

type streamingTransport struct {
	next http.RoundTripper
}

func (rt *streamingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	ctx := request.Context()

	originalRemoteIP, ok := ctx.Value(OriginalRemoteIPContextKey{}).(string)
	if ok {
		request.Header.Add("X-Forwarded-For", originalRemoteIP)
	}
	request.Header.Add("User-Agent", DefaultUserAgent)

	start := time.Now()

	response, err := rt.next.RoundTrip(request)

	fields := log.Fields{
		"method":      request.Method,
		"url":         request.URL.String(),
		"duration_ms": time.Since(start) / time.Millisecond,
	}
	logger := log.WithContextFields(ctx, fields)

	if err != nil {
		logger.WithError(err).Error("Streaming request failed")
		return response, err
	}

	logger = logger.WithField("status", response.StatusCode)

	if response.ContentLength >= 0 {
		logger = logger.WithField("content_length_bytes", response.ContentLength)
	}

	logger.Info("Finished streaming HTTP request")

	return response, nil
}

// NewStreamingTransport creates a transport for long-lived streaming connections that does not
// set request.Close, avoiding HTTP/2 stream resets during full-duplex communication.
func NewStreamingTransport(next http.RoundTripper) http.RoundTripper {
	t := &streamingTransport{next: next}
	return correlation.NewInstrumentedRoundTripper(tracing.NewRoundTripper(t))
}
