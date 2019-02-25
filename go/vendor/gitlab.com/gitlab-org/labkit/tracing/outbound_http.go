package tracing

import (
	"crypto/tls"
	"log"
	"net/http"
	"net/http/httptrace"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	otlog "github.com/opentracing/opentracing-go/log"
)

type tracingRoundTripper struct {
	delegate http.RoundTripper
	config   roundTripperConfig
}

func (c tracingRoundTripper) RoundTrip(req *http.Request) (res *http.Response, e error) {
	tracer := opentracing.GlobalTracer()
	if tracer == nil {
		return c.delegate.RoundTrip(req)
	}

	ctx := req.Context()

	var parentCtx opentracing.SpanContext
	parentSpan := opentracing.SpanFromContext(ctx)
	if parentSpan != nil {
		parentCtx = parentSpan.Context()
	}

	// start a new Span to wrap HTTP request
	span := opentracing.StartSpan(
		c.config.getOperationName(req),
		opentracing.ChildOf(parentCtx),
	)
	defer span.Finish()

	ctx = opentracing.ContextWithSpan(ctx, span)

	// attach ClientTrace to the Context, and Context to request
	trace := newClientTrace(span)
	ctx = httptrace.WithClientTrace(ctx, trace)
	req = req.WithContext(ctx)

	ext.SpanKindRPCClient.Set(span)
	ext.HTTPUrl.Set(span, req.URL.String())
	ext.HTTPMethod.Set(span, req.Method)

	carrier := opentracing.HTTPHeadersCarrier(req.Header)
	err := span.Tracer().Inject(span.Context(), opentracing.HTTPHeaders, carrier)

	if err != nil {
		log.Printf("tracing span injection failed: %v", err)
	}

	response, err := c.delegate.RoundTrip(req)

	if err != nil {
		span.LogFields(
			otlog.String("event", "roundtrip error"),
			otlog.Object("error", err),
		)
	} else {
		span.LogFields(
			otlog.String("event", "roundtrip complete"),
			otlog.Int("status", response.StatusCode),
		)
	}

	return response, err
}

func newClientTrace(span opentracing.Span) *httptrace.ClientTrace {
	trace := &clientTrace{span: span}
	return &httptrace.ClientTrace{
		GotFirstResponseByte: trace.gotFirstResponseByte,
		ConnectStart:         trace.connectStart,
		ConnectDone:          trace.connectDone,
		TLSHandshakeStart:    trace.tlsHandshakeStart,
		TLSHandshakeDone:     trace.tlsHandshakeDone,
		WroteHeaders:         trace.wroteHeaders,
		WroteRequest:         trace.wroteRequest,
	}
}

// clientTrace holds a reference to the Span and
// provides methods used as ClientTrace callbacks
type clientTrace struct {
	span opentracing.Span
}

func (h *clientTrace) gotFirstResponseByte() {
	h.span.LogFields(otlog.String("event", "got first response byte"))
}

func (h *clientTrace) connectStart(network, addr string) {
	h.span.LogFields(
		otlog.String("event", "connect started"),
		otlog.String("network", network),
		otlog.String("addr", addr),
	)
}

func (h *clientTrace) connectDone(network, addr string, err error) {
	h.span.LogFields(
		otlog.String("event", "connect done"),
		otlog.String("network", network),
		otlog.String("addr", addr),
		otlog.Object("error", err),
	)
}

func (h *clientTrace) tlsHandshakeStart() {
	h.span.LogFields(otlog.String("event", "tls handshake started"))
}

func (h *clientTrace) tlsHandshakeDone(state tls.ConnectionState, err error) {
	h.span.LogFields(
		otlog.String("event", "tls handshake done"),
		otlog.Object("error", err),
	)
}

func (h *clientTrace) wroteHeaders() {
	h.span.LogFields(otlog.String("event", "headers written"))
}

func (h *clientTrace) wroteRequest(info httptrace.WroteRequestInfo) {
	h.span.LogFields(
		otlog.String("event", "request written"),
		otlog.Object("error", info.Err),
	)
}

// NewRoundTripper acts as a "client-middleware" for outbound http requests
// adding instrumentation to the outbound request and then delegating to the underlying
// transport
func NewRoundTripper(delegate http.RoundTripper, opts ...RoundTripperOption) http.RoundTripper {
	config := applyRoundTripperOptions(opts)
	return &tracingRoundTripper{delegate: delegate, config: config}
}
