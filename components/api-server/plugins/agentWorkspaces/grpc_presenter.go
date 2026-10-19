package agentWorkspaces

import (
	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// agentWorkspaceToProto converts the stored model into the gRPC watch/list/get
// representation. It mirrors the REST presenter's field list (presenter.go).
func agentWorkspaceToProto(ws *AgentWorkspace) *pb.AgentWorkspace {
	return &pb.AgentWorkspace{
		Metadata: &pb.ObjectReference{
			Id:        ws.ID,
			CreatedAt: timestamppb.New(ws.CreatedAt),
			UpdatedAt: timestamppb.New(ws.UpdatedAt),
			Kind:      "AgentWorkspace",
			Href:      "/api/hypershell/ext/agent_workspaces/" + ws.ID,
		},
		AgentRuntimeId: ws.AgentRuntimeId,
		GatewayId:      ws.GatewayId,
		Name:           ws.Name,
		Status:         ws.Status,
	}
}
