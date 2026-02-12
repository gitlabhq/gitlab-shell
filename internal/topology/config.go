// Package topology provides a client for interacting with the GitLab Cells Topology Service.
//
// The Topology Service is a gRPC server that provides cell routing information for
// requests related to specific records (projects, groups, etc.) in a GitLab Cells
// architecture.
//
// Configuration is done via the topology_service section in config.yml:
//
//	topology_service:
//	  enabled: true
//	  address: "topology.gitlab.com:443"
//	  classify_type: "first_cell"
//	  tls:
//	    enabled: true
//	    ca_file: "/path/to/ca.crt"
//
// For more details, see:
//   - https://handbook.gitlab.com/handbook/engineering/architecture/design-documents/cells/topology_service/
//   - https://gitlab.com/gitlab-org/cells/topology-service
package topology

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// ValidClassifyTypes contains the list of valid classify_type values.
// These correspond to the ClassifyType enum in the Topology Service proto.
var ValidClassifyTypes = []string{"first_cell", "session_prefix", "cell_id"}

// DefaultTimeout is the default timeout for Topology Service requests.
const DefaultTimeout = 5 * time.Second

// Config contains Topology Service client configuration settings.
type Config struct {
	// Enabled indicates whether Topology Service integration is enabled.
	Enabled bool `yaml:"enabled"`

	// Address is the gRPC address of the Topology Service (e.g., "topology.gitlab.com:443").
	Address string `yaml:"address"`

	// ClassifyType specifies which ClassifyType to use when querying the service.
	// Valid values: "first_cell", "session_prefix", "cell_id".
	// Default: "first_cell" (applied at runtime when empty).
	ClassifyType string `yaml:"classify_type"`

	// Timeout is the maximum duration to wait for a response from the Topology Service.
	// Default: 5s (when zero).
	Timeout time.Duration `yaml:"timeout"`

	// TLS contains TLS configuration for secure connections.
	TLS TLSConfig `yaml:"tls"`
}

// TLSConfig contains TLS settings for the Topology Service connection.
type TLSConfig struct {
	// Enabled indicates whether TLS should be used for the connection.
	Enabled bool `yaml:"enabled"`

	// CAFile is the path to the CA certificate file for server verification.
	// If empty, system CA certificates will be used.
	CAFile string `yaml:"ca_file"`

	// CertFile is the path to the client certificate file (for mTLS).
	// Must be provided together with KeyFile.
	CertFile string `yaml:"cert_file"`

	// KeyFile is the path to the client key file (for mTLS).
	// Must be provided together with CertFile.
	KeyFile string `yaml:"key_file"`

	// ServerName is the expected server name for TLS verification.
	// If empty, the hostname from Address will be used.
	ServerName string `yaml:"server_name"`

	// InsecureSkipVerify skips TLS certificate verification.
	// WARNING: This should only be used for development/testing.
	InsecureSkipVerify bool `yaml:"insecure_skip_verify"`
}

// Validate validates the Topology Service configuration.
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil
	}

	if c.Address == "" {
		return errors.New("topology_service.address is required when enabled")
	}

	if !strings.Contains(c.Address, ":") {
		return errors.New("topology_service.address must be in host:port format")
	}

	if c.ClassifyType != "" && !slices.Contains(ValidClassifyTypes, c.ClassifyType) {
		return fmt.Errorf("invalid topology_service.classify_type: %q, must be one of %v", c.ClassifyType, ValidClassifyTypes)
	}

	if err := c.TLS.Validate(); err != nil {
		return fmt.Errorf("topology_service.tls: %w", err)
	}

	return nil
}

// Validate validates the TLS configuration.
func (c *TLSConfig) Validate() error {
	if !c.Enabled {
		return nil
	}

	// Check that both cert and key are provided together for mTLS
	hasCert := c.CertFile != ""
	hasKey := c.KeyFile != ""

	if hasCert != hasKey {
		return errors.New("both cert_file and key_file must be provided for mTLS")
	}

	return nil
}
