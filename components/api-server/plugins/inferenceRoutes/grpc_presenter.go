package inferenceRoutes

import (
	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// inferenceRouteToProto converts the stored model into the gRPC watch/list/get
// representation consumed by the control-plane declarative-import reconciler. It
// mirrors the REST presenter's field list (presenter.go).
func inferenceRouteToProto(ir *InferenceRoute) *pb.InferenceRoute {
	return &pb.InferenceRoute{
		Metadata: &pb.ObjectReference{
			Id:        ir.ID,
			CreatedAt: timestamppb.New(ir.CreatedAt),
			UpdatedAt: timestamppb.New(ir.UpdatedAt),
			Kind:      "InferenceRoute",
			Href:      "/api/hypershell/ext/inference_routes/" + ir.ID,
		},
		WorkspaceId:       ir.WorkspaceId,
		ProviderBindingId: ir.ProviderBindingId,
		Model:             ir.Model,
		RepositoryId:      ir.RepositoryId,
	}
}
