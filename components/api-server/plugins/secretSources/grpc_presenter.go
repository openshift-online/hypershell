package secretSources

import (
	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// secretSourceToProto converts the stored model into the full gRPC
// representation used by the SecretSourceService CRUD/watch surface. The shared
// pb.SecretSource message is also embedded (as a subset) in the AgentRuntime watch
// payload; this path populates the metadata-bearing fields as well. It never
// carries a raw secret value - the model holds only the reference (data-model.spec.md
// "SecretSource" and "Secret Submission and Storage").
func secretSourceToProto(s *SecretSource) *pb.SecretSource {
	return &pb.SecretSource{
		Metadata: &pb.ObjectReference{
			Id:        s.ID,
			CreatedAt: timestamppb.New(s.CreatedAt),
			UpdatedAt: timestamppb.New(s.UpdatedAt),
			Kind:      "SecretSource",
			Href:      "/api/hypershell/ext/secret_sources/" + s.ID,
		},
		Name:           s.Name,
		AgentRuntimeId: s.AgentRuntimeId,
		Purpose:        s.Purpose,
		Backend:        s.Backend,
		Path:           s.Path,
		KeyMappings:    s.KeyMappings,
		RepositoryId:   s.RepositoryId,
	}
}
