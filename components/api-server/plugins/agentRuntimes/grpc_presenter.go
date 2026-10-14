package agentRuntimes

import (
	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// agentRuntimeToProto converts the stored model into the gRPC watch/list
// representation. Beyond the REST presenter's field list (presenter.go) it carries
// two control-plane-facing additions that are gRPC-only (not yet on the REST/OpenAPI
// surface): server-owned StatusMessage, so the control plane observes the current
// status detail, and the owned SecretSources, embedded so the reconciler can render
// one ExternalSecret per source without a second round trip (data-model.spec.md
// "AgentRuntime Cluster Resource Provisioning"). Callers that cannot resolve the
// sources pass nil.
func agentRuntimeToProto(ar *AgentRuntime, sources []SecretSourceView) *pb.AgentRuntime {
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
		StatusMessage:       ar.StatusMessage,
	}
	for _, s := range sources {
		p.SecretSources = append(p.SecretSources, &pb.SecretSource{
			Name:        s.Name,
			Purpose:     s.Purpose,
			Backend:     s.Backend,
			Path:        s.Path,
			KeyMappings: s.KeyMappings,
		})
	}
	return p
}
