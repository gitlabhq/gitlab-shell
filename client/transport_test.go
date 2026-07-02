package client

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/labkit/v2/log"
)

type capturedRecord struct {
	Level   slog.Level
	Message string
}

type capturingHandler struct {
	mu      *sync.Mutex
	records *[]capturedRecord
}

func newCapturingHandler() (*capturingHandler, *[]capturedRecord) {
	records := &[]capturedRecord{}
	return &capturingHandler{mu: &sync.Mutex{}, records: records}, records
}

func (h *capturingHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *capturingHandler) Handle(_ context.Context, record slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	*h.records = append(*h.records, capturedRecord{Level: record.Level, Message: record.Message})
	return nil
}

func (h *capturingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }

func (h *capturingHandler) WithGroup(_ string) slog.Handler { return h }

func hasRecord(records []capturedRecord, level slog.Level, message string) bool {
	for _, r := range records {
		if r.Level == level && r.Message == message {
			return true
		}
	}
	return false
}

const (
	msgInternalAPIError = "Internal API error"
	msgClientError      = "Internal API returned a client error"
	msgFinishedRequest  = "Finished HTTP request"
)

func TestRoundTripLogLevel(t *testing.T) {
	testCases := []struct {
		name            string
		statusCode      int
		body            string
		wantErrorLog    bool
		wantNonErrorMsg string
	}{
		{
			name:            "200 success logs info",
			statusCode:      http.StatusOK,
			body:            `{}`,
			wantNonErrorMsg: msgFinishedRequest,
		},
		{
			name:         "301 redirect logs error",
			statusCode:   http.StatusMovedPermanently,
			wantErrorLog: true,
		},
		{
			name:         "302 redirect logs error",
			statusCode:   http.StatusFound,
			wantErrorLog: true,
		},
		{
			name:         "303 redirect logs error",
			statusCode:   http.StatusSeeOther,
			wantErrorLog: true,
		},
		{
			name:         "307 redirect logs error",
			statusCode:   http.StatusTemporaryRedirect,
			wantErrorLog: true,
		},
		{
			name:         "308 redirect logs error",
			statusCode:   http.StatusPermanentRedirect,
			wantErrorLog: true,
		},
		{
			name:            "403 access denied logs info, not error",
			statusCode:      http.StatusForbidden,
			body:            `{"message":"You are not allowed to push"}`,
			wantNonErrorMsg: msgClientError,
		},
		{
			name:            "404 key not found logs info, not error",
			statusCode:      http.StatusNotFound,
			body:            `{"message":"404 Key Not Found"}`,
			wantNonErrorMsg: msgClientError,
		},
		{
			name:            "404 with non-JSON body logs info at transport level",
			statusCode:      http.StatusNotFound,
			body:            `<html>not found</html>`,
			wantNonErrorMsg: msgClientError,
		},
		{
			name:         "500 logs error",
			statusCode:   http.StatusInternalServerError,
			wantErrorLog: true,
		},
		{
			name:         "502 logs error",
			statusCode:   http.StatusBadGateway,
			wantErrorLog: true,
		},
		{
			name:         "503 logs error",
			statusCode:   http.StatusServiceUnavailable,
			wantErrorLog: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.statusCode)
				if tc.body != "" {
					_, _ = w.Write([]byte(tc.body))
				}
			}))
			defer server.Close()

			records := doRoundTrip(t, server.URL)

			require.Equal(t, tc.wantErrorLog, hasRecord(*records, slog.LevelError, msgInternalAPIError),
				"unexpected error-level log presence")
			if tc.wantErrorLog {
				require.False(t, hasRecord(*records, slog.LevelInfo, msgClientError),
					"expected no client-error info log")
				require.False(t, hasRecord(*records, slog.LevelInfo, msgFinishedRequest),
					"expected no finished-request info log")
			}
			if tc.wantNonErrorMsg != "" {
				require.True(t, hasRecord(*records, slog.LevelInfo, tc.wantNonErrorMsg),
					"expected info-level %q", tc.wantNonErrorMsg)
				require.False(t, hasRecord(*records, slog.LevelError, msgInternalAPIError),
					"expected no error-level log")
			}
		})
	}
}

func TestRoundTripDialErrorLogsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	unreachableURL := server.URL
	server.Close()

	records := doRoundTrip(t, unreachableURL)

	require.True(t, hasRecord(*records, slog.LevelError, "Internal API unreachable"))
	require.False(t, hasRecord(*records, slog.LevelError, msgInternalAPIError))
}

func doRoundTrip(t *testing.T, url string) *[]capturedRecord {
	t.Helper()

	handler, records := newCapturingHandler()
	ctx := log.WithLogger(context.Background(), slog.New(handler))

	transport := NewTransport(DefaultTransport())
	c := retryablehttp.NewClient()
	c.Logger = nil
	c.RetryMax = 0
	c.HTTPClient.Transport = transport
	c.HTTPClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}

	request, err := retryablehttp.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	require.NoError(t, err)

	response, err := c.Do(request)
	if err == nil {
		require.NoError(t, response.Body.Close())
	}

	return records
}
