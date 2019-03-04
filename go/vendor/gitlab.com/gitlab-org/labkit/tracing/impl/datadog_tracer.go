// +build tracer_static,tracer_static_datadog

package impl

import (
	"io"

	opentracing "github.com/opentracing/opentracing-go"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/opentracer"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

func tracerFactory(config map[string]string) (opentracing.Tracer, io.Closer, error) {
	opts := []tracer.StartOption{}
	if config["ServiceName"] != "" {
		opts = append(opts, tracer.WithServiceName(config["ServiceName"]))
	}

	return opentracer.New(opts...), nil, nil
}

func init() {
	registerTracer("datadog", tracerFactory)
}
