package repositories

import (
	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// repositoryToProto converts the stored model into the gRPC watch/list/get
// representation consumed by the control-plane declarative-import reconciler. It
// mirrors the REST presenter's field list (presenter.go).
func repositoryToProto(r *Repository) *pb.Repository {
	return &pb.Repository{
		Metadata: &pb.ObjectReference{
			Id:        r.ID,
			CreatedAt: timestamppb.New(r.CreatedAt),
			UpdatedAt: timestamppb.New(r.UpdatedAt),
			Kind:      "Repository",
			Href:      "/api/hypershell/ext/repositories/" + r.ID,
		},
		Name:           r.Name,
		Url:            r.Url,
		Provider:       r.Provider,
		SecretSourceId: r.SecretSourceId,
		DefaultBranch:  r.DefaultBranch,
		ImportPath:     r.ImportPath,
		Status:         r.Status,
	}
}
