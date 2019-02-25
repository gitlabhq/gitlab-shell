package correlation

// The configuration for InjectCorrelationID
type instrumentedRoundTripperConfig struct {
}

// InstrumentedRoundTripperOption will configure a correlation handler
// currently there are no options, but this gives us the option
// to extend the interface in a backwards compatible way
type InstrumentedRoundTripperOption func(*instrumentedRoundTripperConfig)

func applyInstrumentedRoundTripperOptions(opts []InstrumentedRoundTripperOption) instrumentedRoundTripperConfig {
	config := instrumentedRoundTripperConfig{}
	for _, v := range opts {
		v(&config)
	}

	return config
}
