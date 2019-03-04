package tracing

import (
	"fmt"
	"net/http"
)

// OperationNamer will return an operation name given an HTTP request
type OperationNamer func(*http.Request) string

// The configuration for InjectCorrelationID
type handlerConfig struct {
	getOperationName OperationNamer
}

// HandlerOption will configure a correlation handler
type HandlerOption func(*handlerConfig)

func applyHandlerOptions(opts []HandlerOption) handlerConfig {
	config := handlerConfig{
		getOperationName: func(req *http.Request) string {
			// By default use `GET /x/y/z` for operation names
			return fmt.Sprintf("%s %s", req.Method, req.URL.Path)
		},
	}
	for _, v := range opts {
		v(&config)
	}

	return config
}

// WithRouteIdentifier allows a RouteIdentifier attribute to be set in the handler.
// This value will appear in the traces
func WithRouteIdentifier(routeIdentifier string) HandlerOption {
	return func(config *handlerConfig) {
		config.getOperationName = func(req *http.Request) string {
			// Use `GET routeIdentifier` for operation names
			return fmt.Sprintf("%s %s", req.Method, routeIdentifier)
		}
	}
}
