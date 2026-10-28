package gateways

import (
	"encoding/json"
	"log/slog"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api/presenters"
)

// ConvertGateway maps a user create request to a Gateway model. The create contract is
// restricted to the user-writable allowlist -- name, plus placement/cluster_id resolved
// by the handler before persistence (PLAT-DATA-23). Every other gateway field is
// platform-managed and set only by the control plane, so none is copied here.
func ConvertGateway(gateway openapi.GatewayCreateRequest) *Gateway {
	return &Gateway{Name: gateway.Name, DedicatedRoute: gateway.DedicatedRoute}
}

// PresentGateway converts a Gateway model to its REST representation.
func PresentGateway(gateway *Gateway, createdBy string, canDelete bool, canEdit bool) openapi.Gateway {
	reference := presenters.PresentReference(gateway.ID, gateway)
	g := openapi.Gateway{
		CanDelete:          openapi.PtrBool(canDelete),
		CanEdit:            openapi.PtrBool(canEdit),
		Id:                 reference.Id,
		Kind:               reference.Kind,
		Href:               reference.Href,
		CreatedAt:          openapi.PtrTime(gateway.CreatedAt),
		UpdatedAt:          openapi.PtrTime(gateway.UpdatedAt),
		Name:               gateway.Name,
		ClusterId:          gateway.ClusterId,
		Namespace:          gateway.Namespace,
		ExternalDns:        gateway.ExternalDns,
		TlsMode:            gateway.TlsMode,
		ServiceType:        gateway.ServiceType,
		Status:             gateway.Status,
		Phase:              gateway.Phase,
		Image:              gateway.Image,
		SupervisorImage:    gateway.SupervisorImage,
		RouteAddress:       gateway.RouteAddress,
		ConsoleAddress:     gateway.ConsoleAddress,
		GatewayVersion:     gateway.GatewayVersion,
		Oidc:               gateway.Oidc,
		Route:              gateway.Route,
		DedicatedRoute:     gateway.DedicatedRoute,
		CredentialDriver:   gateway.CredentialDriver,
		WorkspaceMode:      gateway.WorkspaceMode,
		RepositoryId:       gateway.RepositoryId,
		Generation:         &gateway.Generation,
		ObservedGeneration: &gateway.ObservedGeneration,
		ActiveSandboxCount: func() *int32 {
			if gateway.ActiveSandboxCount != nil {
				return openapi.PtrInt32(int32(*gateway.ActiveSandboxCount))
			}
			return nil
		}(),
	}

	if createdBy != "" {
		g.CreatedBy = &createdBy
	}

	if gateway.ServerDnsNames != nil {
		var names []string
		if err := json.Unmarshal([]byte(*gateway.ServerDnsNames), &names); err == nil {
			g.ServerDnsNames = names
		}
	}

	if gateway.ProvisioningConditions != nil {
		var conditions []struct {
			Type            string `json:"type"`
			ConditionStatus string `json:"condition_status"`
			Message         string `json:"message"`
		}
		if err := json.Unmarshal([]byte(*gateway.ProvisioningConditions), &conditions); err != nil {
			slog.Warn("failed to unmarshal provisioning_conditions", "gateway_id", gateway.ID, "error", err)
		} else {
			apiConditions := make([]openapi.GatewayAllOfProvisioningConditions, 0, len(conditions))
			for _, c := range conditions {
				apiConditions = append(apiConditions, openapi.GatewayAllOfProvisioningConditions{
					Type:            &c.Type,
					ConditionStatus: &c.ConditionStatus,
					Message:         &c.Message,
				})
			}
			g.ProvisioningConditions = apiConditions
		}
	}

	return g
}
