package sandboxTemplates

import (
	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// sandboxTemplateToProto converts the stored model into the gRPC
// watch/list/get representation consumed by the control-plane declarative-import
// reconciler. It mirrors the REST presenter's field list (presenter.go).
func sandboxTemplateToProto(st *SandboxTemplate) *pb.SandboxTemplate {
	return &pb.SandboxTemplate{
		Metadata: &pb.ObjectReference{
			Id:        st.ID,
			CreatedAt: timestamppb.New(st.CreatedAt),
			UpdatedAt: timestamppb.New(st.UpdatedAt),
			Kind:      "SandboxTemplate",
			Href:      "/api/hypershell/ext/sandbox_templates/" + st.ID,
		},
		Name:         st.Name,
		Image:        st.Image,
		NamePrefix:   st.NamePrefix,
		Policy:       st.Policy,
		RepositoryId: st.RepositoryId,
		Status:       st.Status,
	}
}
