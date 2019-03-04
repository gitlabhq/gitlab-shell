package tracing

type envInjectorConfig struct{}

// EnvInjectorOption will configure an environment injector
type EnvInjectorOption func(*envInjectorConfig)

func applyEnvInjectorOptions(opts []EnvInjectorOption) envInjectorConfig {
	config := envInjectorConfig{}
	for _, v := range opts {
		v(&config)
	}

	return config
}
