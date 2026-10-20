package workspaceMemberships

import (
	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// workspaceMembershipToProto converts the stored model into the gRPC
// watch/list/get representation. It mirrors the REST presenter's field list
// (presenter.go).
func workspaceMembershipToProto(wm *WorkspaceMembership) *pb.WorkspaceMembership {
	return &pb.WorkspaceMembership{
		Metadata: &pb.ObjectReference{
			Id:        wm.ID,
			CreatedAt: timestamppb.New(wm.CreatedAt),
			UpdatedAt: timestamppb.New(wm.UpdatedAt),
			Kind:      "WorkspaceMembership",
			Href:      "/api/hypershell/ext/workspace_memberships/" + wm.ID,
		},
		WorkspaceId: wm.WorkspaceId,
		Subject:     wm.Subject,
		Role:        wm.Role,
	}
}
