package gateways

import (
	"encoding/json"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/pkg/api/presenters"
)

func ConvertGateway(gateway openapi.GatewayCreateRequest) *Gateway {
	c := &Gateway{}
	c.Name = gateway.Name
	c.FleetId = gateway.FleetId
	c.ClusterId = gateway.ClusterId
	c.ReleaseId = gateway.ReleaseId
	// database_id is assigned by the configured server-side placement strategy.
	// Deliberately ignore any value supplied by the API client.
	c.ExternalDns = gateway.ExternalDns
	c.TlsMode = gateway.TlsMode
	c.ServiceType = gateway.ServiceType
	c.Status = gateway.Status
	c.Phase = gateway.Phase
	c.Image = gateway.Image
	c.SupervisorImage = gateway.SupervisorImage
	c.Oidc = gateway.Oidc
	c.Route = gateway.Route
	c.CredentialDriver = gateway.CredentialDriver

	if len(gateway.ServerDnsNames) > 0 {
		data, _ := json.Marshal(gateway.ServerDnsNames)
		s := string(data)
		c.ServerDnsNames = &s
	}

	return c
}

func PresentGateway(gateway *Gateway, createdBy string) openapi.Gateway {
	reference := presenters.PresentReference(gateway.ID, gateway)
	g := openapi.Gateway{
		Id:               reference.Id,
		Kind:             reference.Kind,
		Href:             reference.Href,
		CreatedAt:        openapi.PtrTime(gateway.CreatedAt),
		UpdatedAt:        openapi.PtrTime(gateway.UpdatedAt),
		Name:             gateway.Name,
		FleetId:          gateway.FleetId,
		ClusterId:        gateway.ClusterId,
		ReleaseId:        gateway.ReleaseId,
		DatabaseId:       gateway.DatabaseId,
		Namespace:        gateway.Namespace,
		ExternalDns:      gateway.ExternalDns,
		TlsMode:          gateway.TlsMode,
		ServiceType:      gateway.ServiceType,
		Status:           gateway.Status,
		Phase:            gateway.Phase,
		Image:            gateway.Image,
		SupervisorImage:  gateway.SupervisorImage,
		RouteAddress:     gateway.RouteAddress,
		ConsoleAddress:   gateway.ConsoleAddress,
		GatewayVersion:   gateway.GatewayVersion,
		Oidc:             gateway.Oidc,
		Route:            gateway.Route,
		CredentialDriver: gateway.CredentialDriver,
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

	return g
}
