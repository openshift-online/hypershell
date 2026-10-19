package agentRuntimes

import (
	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// agentRuntimeToProto converts the stored model into the gRPC watch/list
// representation. Beyond the REST presenter's field list (presenter.go) it carries
// control-plane-facing additions that are gRPC-only (not on the REST/OpenAPI
// surface): server-owned StatusMessage, the owned SecretSources, and the embedded
// gateway-side provisioning inputs (resolved workspace name, ProviderBindings,
// WorkspaceMemberships, and the InferenceRoute) so the reconciler can provision the
// workspace from one payload without a second round trip (data-model.spec.md
// "AgentRuntime Cluster Resource Provisioning", "AgentRuntime Reconcile Payload
// Completeness"). Callers that cannot resolve the sources or the workspace pass nil.
func agentRuntimeToProto(ar *AgentRuntime, sources []SecretSourceView, ws *WorkspaceView) *pb.AgentRuntime {
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
	embedWorkspace(p, ws)
	return p
}

// embedWorkspace populates the workspace-derived payload fields (name, provider
// bindings, memberships, inference route) from a resolved WorkspaceView. A nil view
// leaves them unset, so a runtime without a provisioned workspace still streams.
func embedWorkspace(p *pb.AgentRuntime, ws *WorkspaceView) {
	if ws == nil {
		return
	}
	if ws.Name != "" {
		name := ws.Name
		p.WorkspaceName = &name
	}
	for _, b := range ws.ProviderBindings {
		pbMsg := &pb.ProviderBinding{
			Metadata: &pb.ObjectReference{
				Id:   b.ID,
				Kind: "ProviderBinding",
				Href: "/api/hypershell/ext/provider_bindings/" + b.ID,
			},
			Name:            b.Name,
			WorkspaceId:     b.WorkspaceID,
			ProviderSpecId:  b.ProviderSpecID,
			RefreshStrategy: b.RefreshStrategy,
			Status:          b.Status,
			SecretSourceId:  b.SecretSourceID,
		}
		p.ProviderBindings = append(p.ProviderBindings, pbMsg)
	}
	for _, m := range ws.Memberships {
		p.WorkspaceMemberships = append(p.WorkspaceMemberships, &pb.WorkspaceMembership{
			Metadata: &pb.ObjectReference{
				Id:   m.ID,
				Kind: "WorkspaceMembership",
				Href: "/api/hypershell/ext/workspace_memberships/" + m.ID,
			},
			WorkspaceId: m.WorkspaceID,
			Subject:     m.Subject,
			Role:        m.Role,
		})
	}
	if ws.Route != nil {
		p.InferenceRoute = &pb.InferenceRoute{
			Metadata: &pb.ObjectReference{
				Id:   ws.Route.ID,
				Kind: "InferenceRoute",
				Href: "/api/hypershell/ext/inference_routes/" + ws.Route.ID,
			},
			WorkspaceId:       ws.Route.WorkspaceID,
			ProviderBindingId: ws.Route.ProviderBindingID,
			Model:             ws.Route.Model,
		}
	}
}
