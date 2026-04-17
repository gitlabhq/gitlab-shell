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
func SSHKeyClaim(key string) *types_proto.Claim {
	return &types_proto.Claim{Claim: &types_proto.Claim_SshKey{SshKey: key}}
}

// ProjectIDClaim creates a Claim for a project ID.
func ProjectIDClaim(id int64) *types_proto.Claim {
	return &types_proto.Claim{Claim: &types_proto.Claim_ProjectId{ProjectId: id}}
}
