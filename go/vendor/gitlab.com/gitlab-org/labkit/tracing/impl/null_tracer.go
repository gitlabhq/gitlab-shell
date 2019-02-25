// +build !tracer_static

package impl

import (
	"fmt"
	"io"

	opentracing "github.com/opentracing/opentracing-go"
)

// New will instantiate a new instance of the tracer, given the driver and configuration
func New(driverName string, config map[string]string) (opentracing.Tracer, io.Closer, error) {
	return nil, nil, fmt.Errorf("tracer: binary compiled without tracer support: cannot load driver %s", driverName)
}
