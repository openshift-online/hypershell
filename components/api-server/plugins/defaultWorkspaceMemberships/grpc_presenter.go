package defaultWorkspaceMemberships

import (
	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// toProto converts a DefaultWorkspaceMembership to its gRPC representation.
// metadata.href is intentionally empty: this Kind has no REST path (PLAT-DATA-27,
// "No public CRUD").
func toProto(m *DefaultWorkspaceMembership) *pb.DefaultWorkspaceMembership {
	p := &pb.DefaultWorkspaceMembership{
		Metadata: &pb.ObjectReference{
			Id:        m.ID,
			CreatedAt: timestamppb.New(m.CreatedAt),
			UpdatedAt: timestamppb.New(m.UpdatedAt),
			Kind:      "DefaultWorkspaceMembership",
		},
		GatewayId:            m.GatewayID,
		Subject:              m.Subject,
		Role:                 m.Role,
		Source:               m.Source,
		ReconciliationStatus: m.ReconciliationStatus,
	}
	if m.LastReconcileError != nil {
		p.LastReconcileError = m.LastReconcileError
	}
	return p
}
