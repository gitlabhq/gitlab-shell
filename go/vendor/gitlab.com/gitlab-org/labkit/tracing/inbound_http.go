package tracing

import (
	"net/http"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"gitlab.com/gitlab-org/labkit/correlation"
)

// Handler will extract tracing from inbound request
func Handler(h http.Handler, opts ...HandlerOption) http.Handler {
	config := applyHandlerOptions(opts)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tracer := opentracing.GlobalTracer()
		if tracer == nil {
			h.ServeHTTP(w, r)
			return
		}

		wireContext, _ := tracer.Extract(
			opentracing.HTTPHeaders,
			opentracing.HTTPHeadersCarrier(r.Header))

		// Create the span referring to the RPC client if available.
		// If wireContext == nil, a root span will be created.
		additionalStartSpanOpts := []opentracing.StartSpanOption{
			ext.RPCServerOption(wireContext),
		}

		correlationID := correlation.ExtractFromContext(r.Context())
		if correlationID != "" {
			additionalStartSpanOpts = append(additionalStartSpanOpts, opentracing.Tag{Key: "correlation_id", Value: correlationID})
		}

		serverSpan := opentracing.StartSpan(
			config.getOperationName(r),
			additionalStartSpanOpts...,
		)
		defer serverSpan.Finish()

		ctx := opentracing.ContextWithSpan(r.Context(), serverSpan)

		h.ServeHTTP(w, r.WithContext(ctx))
	})

}
