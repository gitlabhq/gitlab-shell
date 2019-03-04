// +build tracer_static

package impl

import (
	"fmt"
	"io"

	opentracing "github.com/opentracing/opentracing-go"
)

// New will instantiate a new instance of the tracer, given the driver and configuration
func New(driverName string, config map[string]string) (opentracing.Tracer, io.Closer, error) {
	factory := registry[driverName]
	if factory == nil {
		return nil, nil, fmt.Errorf("tracer: unable to load driver %s", driverName)
	}

	return factory(config)
}
