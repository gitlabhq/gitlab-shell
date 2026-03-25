package topology

import (
	"testing"

	"github.com/stretchr/testify/require"
	types_proto "gitlab.com/gitlab-org/cells/topology-service/clients/go/proto/types/v1"
)

func TestRouteClaim(t *testing.T) {
	claim := RouteClaim("my-group/my-project")
	require.IsType(t, &types_proto.Claim_Route{}, claim.GetClaim())
	require.Equal(t, "my-group/my-project", claim.GetRoute())
}

func TestSSHKeyClaim(t *testing.T) {
	claim := SSHKeyClaim("ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ")
	require.IsType(t, &types_proto.Claim_SshKey{}, claim.GetClaim())
	require.Equal(t, "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ", claim.GetSshKey())
}

func TestProjectIDClaim(t *testing.T) {
	claim := ProjectIDClaim(42)
	require.IsType(t, &types_proto.Claim_ProjectId{}, claim.GetClaim())
	require.Equal(t, int64(42), claim.GetProjectId())
}
