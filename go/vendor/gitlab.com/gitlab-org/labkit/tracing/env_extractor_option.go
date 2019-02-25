package tracing

type extractFromEnvConfig struct{}

// ExtractFromEnvOption will configure an environment injector
type ExtractFromEnvOption func(*extractFromEnvConfig)

func applyExtractFromEnvOptions(opts []ExtractFromEnvOption) extractFromEnvConfig {
	config := extractFromEnvConfig{}
	for _, v := range opts {
		v(&config)
	}

	return config
}
