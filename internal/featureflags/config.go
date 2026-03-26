// Package featureflags provides configuration for the labkit v2 feature flag
// client (backed by Flipt via OpenFeature).
//
// Configuration is done via the feature_flags section in config.yml:
//
//	feature_flags:
//	  endpoint: "http://flipt:8080"
//	  namespace: "gitlab-shell"
//
// When endpoint is empty, the labkit featureflag client falls back to the
// FEATURE_FLAG_ENDPOINT environment variable internally. If neither is
// provided the client is not started and all flag checks return their default
// values.
package featureflags

// Config contains feature flag client configuration settings.
type Config struct {
	// Endpoint is the HTTP address of the Flipt server
	// (e.g. "http://flipt:8080"). When empty, labkit falls back to the
	// FEATURE_FLAG_ENDPOINT environment variable. When neither is set the
	// client is not started and all flag checks return their default value.
	Endpoint string `yaml:"endpoint"`

	// Namespace is the Flipt namespace used for flag lookup and evaluation.
	// When empty the Flipt provider's own default namespace ("default") is
	// used.
	Namespace string `yaml:"namespace"`
}
