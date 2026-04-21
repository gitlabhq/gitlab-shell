package healthcheck

import (
	"testing"
)

// TestHealthcheck_FeatureFlagMigration documents the expected behavior of the
// feature-flagged healthcheck client migration. These tests should be run against
// an actual GitLab instance with the FEATURE_FLAG_ENDPOINT configured.
//
// Expected behavior:
// 1. With feature flag disabled (default): uses old gitlabnet client
// 2. With feature flag enabled: uses new internal/clients/gitlab client
// 3. Both paths return identical responses for the same GitLab instance
// 4. If flag evaluation fails: gracefully falls back to old client
func TestHealthcheck_FeatureFlagIntegration(t *testing.T) {
	t.Skip("Integration test - requires real GitLab instance and FEATURE_FLAG_ENDPOINT")
}
