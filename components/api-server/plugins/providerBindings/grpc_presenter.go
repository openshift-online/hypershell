package providerBindings

import (
	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// providerBindingToProto converts the stored model into the gRPC
// watch/list/get representation consumed by the control-plane
// declarative-import reconciler. It mirrors the REST presenter's field list
// (presenter.go), including the optional<->string mapping for secret_source_id.
func providerBindingToProto(b *ProviderBinding) *pb.ProviderBinding {
	p := &pb.ProviderBinding{
		Metadata: &pb.ObjectReference{
			Id:        b.ID,
			CreatedAt: timestamppb.New(b.CreatedAt),
			UpdatedAt: timestamppb.New(b.UpdatedAt),
			Kind:      "ProviderBinding",
			Href:      "/api/hypershell/ext/provider_bindings/" + b.ID,
		},
		Name:            b.Name,
		WorkspaceId:     b.WorkspaceId,
		ProviderSpecId:  b.ProviderSpecId,
		RefreshStrategy: b.RefreshStrategy,
		RepositoryId:    b.RepositoryId,
		Status:          b.Status,
	}
	if b.SecretSourceId != "" {
		p.SecretSourceId = &b.SecretSourceId
	}
	return p
}
