package correlation

// The configuration for InjectCorrelationID
type inboundHandlerConfig struct {
	propagation        bool
	sendResponseHeader bool
}

// InboundHandlerOption will configure a correlation handler
// currently there are no options, but this gives us the option
// to extend the interface in a backwards compatible way
type InboundHandlerOption func(*inboundHandlerConfig)

func applyInboundHandlerOptions(opts []InboundHandlerOption) inboundHandlerConfig {
	config := inboundHandlerConfig{
		propagation: false,
	}
	for _, v := range opts {
		v(&config)
	}

	return config
}

// WithPropagation will configure the handler to propagate existing correlation_ids
// passed in from upstream services.
// This is not the default behaviour.
func WithPropagation() InboundHandlerOption {
	return func(config *inboundHandlerConfig) {
		config.propagation = true
	}
}

// WithSetResponseHeader will configure the handler to set the correlation_id
// in the http response headers
func WithSetResponseHeader() InboundHandlerOption {
	return func(config *inboundHandlerConfig) {
		config.sendResponseHeader = true
	}
}
