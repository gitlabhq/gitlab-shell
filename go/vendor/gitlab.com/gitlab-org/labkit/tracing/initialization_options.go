package tracing

import (
	"os"
	"path"
)

const tracingEnvKey = "GITLAB_TRACING"

// The configuration for InjectCorrelationID
type initializationConfig struct {
	serviceName      string
	connectionString string
}

// InitializationOption will configure a correlation handler
type InitializationOption func(*initializationConfig)

func applyInitializationOptions(opts []InitializationOption) initializationConfig {
	config := initializationConfig{
		serviceName:      path.Base(os.Args[0]),
		connectionString: os.Getenv(tracingEnvKey),
	}
	for _, v := range opts {
		v(&config)
	}

	return config
}

// WithServiceName allows the service name to be configured for the tracer
// this will appear in traces.
func WithServiceName(serviceName string) InitializationOption {
	return func(config *initializationConfig) {
		config.serviceName = serviceName
	}
}

// WithConnectionString allows the opentracing connection string to be overridden. By default
// this will be retrieved from the GITLAB_TRACING environment variable.
func WithConnectionString(connectionString string) InitializationOption {
	return func(config *initializationConfig) {
		config.connectionString = connectionString
	}
}
