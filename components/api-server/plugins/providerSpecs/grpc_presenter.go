package providerSpecs

import (
	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// providerSpecToProto converts the stored model into the gRPC watch/list/get
// representation consumed by the control-plane declarative-import reconciler. It
// mirrors the REST presenter's field list (presenter.go).
func providerSpecToProto(ps *ProviderSpec) *pb.ProviderSpec {
	return &pb.ProviderSpec{
		Metadata: &pb.ObjectReference{
			Id:        ps.ID,
			CreatedAt: timestamppb.New(ps.CreatedAt),
			UpdatedAt: timestamppb.New(ps.UpdatedAt),
			Kind:      "ProviderSpec",
			Href:      "/api/hypershell/ext/provider_specs/" + ps.ID,
		},
		Name:         ps.Name,
		Category:     ps.Category,
		Capability:   ps.Capability,
		Profile:      ps.Profile,
		RepositoryId: ps.RepositoryId,
		Status:       ps.Status,
	}
}
