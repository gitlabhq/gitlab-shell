package impl

import (
	"io"

	opentracing "github.com/opentracing/opentracing-go"
)

type tracerFactoryFunc func(config map[string]string) (opentracing.Tracer, io.Closer, error)

var registry = map[string]tracerFactoryFunc{}

func registerTracer(name string, factory tracerFactoryFunc) {
	registry[name] = factory
}
