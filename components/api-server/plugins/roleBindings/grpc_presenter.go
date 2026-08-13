package roleBindings

import (
	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func roleBindingToProto(rb *RoleBinding, roleName string) *pb.RoleBinding {
	p := &pb.RoleBinding{
		Metadata: &pb.ObjectReference{
			Id:        rb.ID,
			CreatedAt: timestamppb.New(rb.CreatedAt),
			UpdatedAt: timestamppb.New(rb.UpdatedAt),
			Kind:      "RoleBinding",
			Href:      "/api/hypershell/v1/role_bindings/" + rb.ID,
		},
		RoleId:   rb.RoleID,
		Scope:    rb.Scope,
		UserId:   rb.UserID,
		RoleName: roleName,
	}

	if rb.GatewayID != nil {
		p.GatewayId = rb.GatewayID
	}

	if rb.UserID != nil {
		p.Username = *rb.UserID
	}

	return p
}
