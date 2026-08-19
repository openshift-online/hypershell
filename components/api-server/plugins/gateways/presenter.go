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
	c.DatabaseId = gateway.DatabaseId
	c.ExternalDns = gateway.ExternalDns
	c.TlsMode = gateway.TlsMode
	c.ServiceType = gateway.ServiceType
	c.Status = gateway.Status
	c.Phase = gateway.Phase
	c.Image = gateway.Image
	c.SupervisorImage = gateway.SupervisorImage
	c.Oidc = gateway.Oidc
	c.Route = gateway.Route
	c.DatabaseConfig = gateway.DatabaseConfig
	c.CredentialDriver = gateway.CredentialDriver

	if len(gateway.ServerDnsNames) > 0 {
		data, _ := json.Marshal(gateway.ServerDnsNames)
		s := string(data)
		c.ServerDnsNames = &s
	}

	return c
}

func PresentGateway(gateway *Gateway) openapi.Gateway {
	reference := presenters.PresentReference(gateway.ID, gateway)
	g := openapi.Gateway{
		Id:                 reference.Id,
		Kind:               reference.Kind,
		Href:               reference.Href,
		CreatedAt:          openapi.PtrTime(gateway.CreatedAt),
		UpdatedAt:          openapi.PtrTime(gateway.UpdatedAt),
		Name:               gateway.Name,
		FleetId:            gateway.FleetId,
		ClusterId:          gateway.ClusterId,
		ReleaseId:          gateway.ReleaseId,
		DatabaseId:         gateway.DatabaseId,
		Namespace:          gateway.Namespace,
		ExternalDns:        gateway.ExternalDns,
		TlsMode:            gateway.TlsMode,
		ServiceType:        gateway.ServiceType,
		Status:             gateway.Status,
		Phase:              gateway.Phase,
		Image:              gateway.Image,
		SupervisorImage:    gateway.SupervisorImage,
		RouteAddress:       gateway.RouteAddress,
		Oidc:               gateway.Oidc,
		Route:              gateway.Route,
		DatabaseConfig:     gateway.DatabaseConfig,
		CredentialDriver:   gateway.CredentialDriver,
		Generation:         &gateway.Generation,
		ObservedGeneration: &gateway.ObservedGeneration,
	}

	if gateway.ServerDnsNames != nil {
		var names []string
		if err := json.Unmarshal([]byte(*gateway.ServerDnsNames), &names); err == nil {
			g.ServerDnsNames = names
		}
	}

	return g
}
