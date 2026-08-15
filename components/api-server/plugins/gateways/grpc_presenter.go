package gateways

import (
	"encoding/json"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func gatewayToProto(d *Gateway) *pb.Gateway {
	gw := &pb.Gateway{
		Metadata: &pb.ObjectReference{
			Id:        d.ID,
			CreatedAt: timestamppb.New(d.CreatedAt),
			UpdatedAt: timestamppb.New(d.UpdatedAt),
			Kind:      "Gateway",
			Href:      "/api/hypershell/v1/gateways/" + d.ID,
		},
		Name:             d.Name,
		FleetId:          d.FleetId,
		ClusterId:        d.ClusterId,
		ReleaseId:        d.ReleaseId,
		DatabaseId:       d.DatabaseId,
		Namespace:        d.Namespace,
		ExternalDns:      d.ExternalDns,
		TlsMode:          d.TlsMode,
		ServiceType:      d.ServiceType,
		Status:           d.Status,
		Phase:            d.Phase,
		Image:            d.Image,
		SupervisorImage:  d.SupervisorImage,
		RouteAddress:     d.RouteAddress,
		Oidc:             d.Oidc,
		Route:            d.Route,
		DatabaseConfig:   d.DatabaseConfig,
		CredentialDriver: d.CredentialDriver,
	}

	if d.ServerDnsNames != nil {
		var names []string
		if err := json.Unmarshal([]byte(*d.ServerDnsNames), &names); err == nil {
			gw.ServerDnsNames = names
		}
	}

	return gw
}
