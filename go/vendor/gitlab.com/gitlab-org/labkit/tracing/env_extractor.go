package tracing

import (
	"context"
	"os"
	"strings"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"gitlab.com/gitlab-org/labkit/correlation"
)

// ExtractFromEnv will extract a span from the environment after it has been passed in
// from the parent process. Returns a new context, and a defer'able function, which
// should be called on process termination
func ExtractFromEnv(ctx context.Context, opts ...ExtractFromEnvOption) (context.Context, func()) {
	/* config not yet used */ applyExtractFromEnvOptions(opts)
	tracer := opentracing.GlobalTracer()

	// Extract the Correlation-ID
	envMap := environAsMap(os.Environ())
	correlationID := envMap[envCorrelationIDKey]
	if correlationID != "" {
		ctx = correlation.ContextWithCorrelation(ctx, correlationID)
	}

	// Attempt to deserialize tracing identifiers
	wireContext, err := tracer.Extract(
		opentracing.TextMap,
		opentracing.TextMapCarrier(envMap))

	if err != nil {
		/* Clients could send bad data, in which case we simply ignore it */
		return ctx, func() {}
	}

	// Create the span referring to the RPC client if available.
	// If wireContext == nil, a root span will be created.
	additionalStartSpanOpts := []opentracing.StartSpanOption{
		ext.RPCServerOption(wireContext),
	}

	if correlationID != "" {
		additionalStartSpanOpts = append(additionalStartSpanOpts, opentracing.Tag{Key: "correlation_id", Value: correlationID})
	}

	serverSpan := opentracing.StartSpan(
		"execute",
		additionalStartSpanOpts...,
	)
	ctx = opentracing.ContextWithSpan(ctx, serverSpan)

	return ctx, func() { serverSpan.Finish() }
}

func environAsMap(env []string) map[string]string {
	envMap := make(map[string]string, len(env))
	for _, v := range env {
		s := strings.SplitN(v, "=", 2)
		envMap[s[0]] = s[1]
	}
	return envMap
}
