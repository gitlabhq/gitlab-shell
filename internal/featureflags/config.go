// Package featureflags provides configuration for the GitLab Shell feature flag client.
// It defines the Config struct that controls how the Flipt-backed feature flag
// client is initialised in Setup().
package featureflags

import "time"

// Config contains feature flag client configuration settings.
type Config struct {
	// Endpoint is the base URL of the Flipt server
	// (e.g. "http://flipt.internal:8080"). When empty, the
	// FEATURE_FLAG_ENDPOINT environment variable is used instead.
	// If neither is set, the feature flag client is disabled and all
	// flag evaluations default to false.
	Endpoint string `yaml:"endpoint"`

	// Namespace is the Flipt namespace to query. Defaults to "gitlab-shell".
	Namespace string `yaml:"namespace"`

	// CacheTTL is how long evaluated flag values are cached locally.
	// Defaults to 60 seconds.
	CacheTTL time.Duration `yaml:"cache_ttl"`

	// CacheSize is the maximum number of flag values held in the local cache.
	// Defaults to 1000.
	CacheSize int `yaml:"cache_size"`

	// RequestTimeout is the HTTP timeout for individual Flipt requests.
	// Defaults to 1 second.
	RequestTimeout time.Duration `yaml:"request_timeout"`
}
