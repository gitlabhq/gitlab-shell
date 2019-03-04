package tracing

import (
	"io"
	"log"

	opentracing "github.com/opentracing/opentracing-go"
	"gitlab.com/gitlab-org/labkit/tracing/connstr"
	"gitlab.com/gitlab-org/labkit/tracing/impl"
)

type nopCloser struct {
}

func (nopCloser) Close() error { return nil }

// Initialize will initialize distributed tracing
func Initialize(opts ...InitializationOption) io.Closer {
	config := applyInitializationOptions(opts)

	if config.connectionString == "" {
		// No opentracing connection has been set
		return &nopCloser{}
	}

	driverName, options, err := connstr.Parse(config.connectionString)
	if err != nil {
		log.Printf("unable to parse connection: %v", err)
		return &nopCloser{}
	}

	if config.serviceName != "" {
		options["ServiceName"] = config.serviceName
	}

	tracer, closer, err := impl.New(driverName, options)
	if err != nil {
		log.Printf("skipping tracing configuration step: %v", err)
		return &nopCloser{}
	}

	if tracer == nil {
		log.Printf("no tracer provided, tracing will be disabled")
	} else {
		log.Printf("Tracing enabled")
		opentracing.SetGlobalTracer(tracer)
	}

	if closer == nil {
		return &nopCloser{}
	}
	return closer
}
