package topology

import (
	types_proto "gitlab.com/gitlab-org/cells/topology-service/clients/go/proto/types/v1"
)

// RouteClaim creates a Claim for a GitLab route (e.g., "my-group/my-project").
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
