package topology

import (
	types_proto "gitlab.com/gitlab-org/cells/topology-service/clients/go/proto/types/v1"
)

// RouteClaim creates a Claim for a GitLab top-level route (e.g., "my-group").
// Only top-level paths (without "/") are claimed in the Topology Service;
// sub-routes like "my-group/my-project" are derived from their parent
// and should not be used as classification keys.
func RouteClaim(route string) *types_proto.Claim {
	return &types_proto.Claim{Claim: &types_proto.Claim_Route{Route: route}}
}

// SSHKeyClaim creates a Claim for an SSH public key.
//
// Deprecated: Use SSHFingerprintClaim instead to classify by SHA-256 fingerprint.
func SSHKeyClaim(key string) *types_proto.Claim {
	return &types_proto.Claim{Claim: &types_proto.Claim_SshKey{SshKey: key}}
}

// SSHFingerprintClaim creates a Claim for an SSH key's SHA-256 fingerprint.
// The fingerprint must be the raw base64 body (43 chars), without the "SHA256:" prefix.
// This matches the keys.fingerprint_sha256 format stored in the GitLab database.
func SSHFingerprintClaim(fingerprint string) *types_proto.Claim {
	return &types_proto.Claim{Claim: &types_proto.Claim_SshKeyFingerprint{SshKeyFingerprint: fingerprint}}
}

// ProjectIDClaim creates a Claim for a project ID.
func ProjectIDClaim(id int64) *types_proto.Claim {
	return &types_proto.Claim{Claim: &types_proto.Claim_ProjectId{ProjectId: id}}
}

// UsernameClaim creates a Claim for a GitLab username.
// Callers should not pass an empty string; the resolver guards against
// this, but calling UsernameClaim("") directly would send an empty
// claim to the Topology Service.
func UsernameClaim(username string) *types_proto.Claim {
	return &types_proto.Claim{Claim: &types_proto.Claim_Username{Username: username}}
}
