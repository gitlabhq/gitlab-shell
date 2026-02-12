package topology

import (
	// Import the Topology Service proto package to ensure the dependency is retained.
	// The client implementation will be added in a subsequent MR.
	pb "gitlab.com/gitlab-org/cells/topology-service/clients/go/proto"
)

// ClassifyType constants mirror the proto enum values for convenience.
const (
	ClassifyTypeFirstCell     = pb.ClassifyType_FIRST_CELL
	ClassifyTypeSessionPrefix = pb.ClassifyType_SESSION_PREFIX
	ClassifyTypeCellID        = pb.ClassifyType_CELL_ID
)
