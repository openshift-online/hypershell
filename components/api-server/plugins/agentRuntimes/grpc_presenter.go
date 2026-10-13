package agentRuntimes

import (
	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// agentRuntimeToProto converts the stored model into the gRPC watch/list
// representation. It mirrors the REST presenter's field list (presenter.go) and
// includes server-owned Status in the outbound direction so the control plane
// observes the current status lifecycle value.
func agentRuntimeToProto(ar *AgentRuntime) *pb.AgentRuntime {
	p := &pb.AgentRuntime{
		Metadata: &pb.ObjectReference{
			Id:        ar.ID,
			CreatedAt: timestamppb.New(ar.CreatedAt),
			UpdatedAt: timestamppb.New(ar.UpdatedAt),
			Kind:      "AgentRuntime",
			Href:      "/api/hypershell/ext/agent_runtimes/" + ar.ID,
		},
		Name:                ar.Name,
		ClusterId:           ar.ClusterId,
		GatewayId:           ar.GatewayId,
		SandboxTemplateId:   ar.SandboxTemplateId,
		RepositoryId:        ar.RepositoryId,
		Selector:            ar.Selector,
		Description:         ar.Description,
		Cron:                ar.Cron,
		CoordinatorImage:    ar.CoordinatorImage,
		ConcurrencyPolicy:   ar.ConcurrencyPolicy,
		LoginRefreshSeconds: ar.LoginRefreshSeconds,
		Parameters:          ar.Parameters,
		Status:              ar.Status,
	}
	return p
}
