// Package featureflags provides configuration for the labkit v2 feature flag
// client (backed by Flipt via OpenFeature).
//
// Configuration is done via the feature_flags section in config.yml:
//
//	feature_flags:
//	  enabled: true
//	  endpoint: "http://flipt:8080"
//	  namespace: "gitlab-shell"
//
// The endpoint can also be supplied via the FEATURE_FLAG_ENDPOINT environment
// variable when it is not set in the YAML file. If neither is provided and
// enabled is true the process will log a warning at startup and continue with
// all flag checks returning their default values.
package featureflags

// Config contains feature flag client configuration settings.
type Config struct {
	// Enabled indicates whether feature flag evaluation is active.
	// When false the client is not started and all flag checks return their
	// default value.
	Enabled bool `yaml:"enabled"`

	// Endpoint is the HTTP address of the Flipt server
	// (e.g. "http://flipt:8080"). When empty the client falls back to the
	// FEATURE_FLAG_ENDPOINT environment variable. Not required when Enabled
	// is false.
	Endpoint string `yaml:"endpoint"`

	// Namespace is the Flipt namespace used for flag lookup and evaluation.
	// When empty the Flipt provider's own default namespace ("default") is
	// used.
	Namespace string `yaml:"namespace"`
}
