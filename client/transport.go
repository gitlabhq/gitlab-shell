// Package client provides an HTTP client with enhanced logging, tracing, and correlation handling.
package client

import (
	"log/slog"
	"net/http"
	"time"

	"gitlab.com/gitlab-org/labkit/correlation"
	"gitlab.com/gitlab-org/labkit/fields"

	"gitlab.com/gitlab-org/labkit/tracing"
	"gitlab.com/gitlab-org/labkit/v2/log"
)

type transport struct {
	next http.RoundTripper
}

// RoundTrip executes a single HTTP transaction, adding logging and tracing capabilities.
func (rt *transport) RoundTrip(request *http.Request) (*http.Response, error) {
	ctx := request.Context()
	logger := log.New()

	originalRemoteIP, ok := ctx.Value(OriginalRemoteIPContextKey{}).(string)
	if ok {
		request.Header.Add("X-Forwarded-For", originalRemoteIP)
	}
	request.Close = true
	request.Header.Add("User-Agent", defaultUserAgent)

	start := time.Now()

	response, err := rt.next.RoundTrip(request)

	ctx = log.WithFields(ctx,
		slog.String("method", request.Method),
		slog.String("url", request.URL.String()),
		slog.Duration("duration", time.Since(start)/time.Millisecond),
	)

	if err != nil {
		logger.ErrorContext(ctx,
			"Internal API unreachable",
			slog.String(fields.ErrorMessage, err.Error()),
		)
		return response, err
	}

	ctx = log.WithFields(ctx,
		slog.Int("status", response.StatusCode),
	)

	if response.StatusCode >= 400 {
		logger.ErrorContext(ctx, "Internal API error")
		return response, nil
	}

	if response.ContentLength >= 0 {
		ctx = log.WithFields(
			ctx,
			slog.Int64("content_length_bytes", response.ContentLength),
		)
	}

	logger.InfoContext(ctx, "Finished HTTP request")
	return response, nil
}

// DefaultTransport returns a clone of the default HTTP transport.
func DefaultTransport() http.RoundTripper {
	return http.DefaultTransport.(*http.Transport).Clone()
}

// NewTransport creates a new transport with logging, tracing, and correlation handling.
func NewTransport(next http.RoundTripper) http.RoundTripper {
	t := &transport{next: next}
	return correlation.NewInstrumentedRoundTripper(tracing.NewRoundTripper(t))
}
