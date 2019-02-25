package tracing

import (
	"fmt"
)

// ErrConfiguration is returned when the tracer is not properly configured.
var ErrConfiguration = fmt.Errorf("Tracing is not properly configured")
