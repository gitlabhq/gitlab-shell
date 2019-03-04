package tracing

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"

	opentracing "github.com/opentracing/opentracing-go"
	"gitlab.com/gitlab-org/labkit/correlation"
)

// envCorrelationIDKey is used to pass the current correlation-id over to the child process
const envCorrelationIDKey = "CORRELATION_ID"

// EnvInjector will inject tracing information into an environment in preparation for
// spawning a child process. This includes trace and span identifiers, as well
// as the GITLAB_TRACING configuration. Will gracefully degrade if tracing is
// not configured, or an active span is not currently available.
type EnvInjector func(ctx context.Context, env []string) []string

// NewEnvInjector will create a new environment injector
func NewEnvInjector(opts ...EnvInjectorOption) EnvInjector {
	/* config not yet used */ applyEnvInjectorOptions(opts)

	return func(ctx context.Context, env []string) []string {
		envMap := map[string]string{}

		// Pass the Correlation-ID through the environment if set
		correlationID := correlation.ExtractFromContext(ctx)
		if correlationID != "" {
			envMap[envCorrelationIDKey] = correlationID
		}

		// Also include the GITLAB_TRACING configuration so that
		// the child process knows how to configure itself
		v, ok := os.LookupEnv(tracingEnvKey)
		if ok {
			envMap[tracingEnvKey] = v
		}

		span := opentracing.SpanFromContext(ctx)
		if span == nil {
			// If no active span, short circuit
			return appendMapToEnv(env, envMap)
		}

		carrier := opentracing.TextMapCarrier(envMap)
		err := span.Tracer().Inject(span.Context(), opentracing.TextMap, carrier)

		if err != nil {
			log.Printf("tracing span injection failed: %v", err)
		}

		return appendMapToEnv(env, envMap)
	}
}

// appendMapToEnv takes a map of key,value pairs and appends it to an
// array of environment variable pairs in `K=V` string pairs
func appendMapToEnv(env []string, envMap map[string]string) []string {
	additionalItems := []string{}
	for k, v := range envMap {
		additionalItems = append(additionalItems, fmt.Sprintf("%s=%s", k, v))
	}

	sort.Strings(additionalItems)
	return append(env, additionalItems...)
}
