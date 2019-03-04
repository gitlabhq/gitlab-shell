package tracing

import (
	"fmt"
	"net/http"
)

// The configuration for InjectCorrelationID
type roundTripperConfig struct {
	getOperationName OperationNamer
}

// RoundTripperOption will configure a correlation handler
type RoundTripperOption func(*roundTripperConfig)

func applyRoundTripperOptions(opts []RoundTripperOption) roundTripperConfig {
	config := roundTripperConfig{
		getOperationName: func(req *http.Request) string {
			// By default use `GET https://localhost` for operation names
			return fmt.Sprintf("%s %s://%s", req.Method, req.URL.Scheme, req.URL.Host)
		},
	}
	for _, v := range opts {
		v(&config)
	}

	return config
}
