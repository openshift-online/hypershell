package gatewayNetworks

import (
	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func gatewayNetworkToProto(d *GatewayNetwork) *pb.GatewayNetwork {
	return &pb.GatewayNetwork{
		Metadata: &pb.ObjectReference{
			Id:        d.ID,
			CreatedAt: timestamppb.New(d.CreatedAt),
			UpdatedAt: timestamppb.New(d.UpdatedAt),
			Kind:      "GatewayNetwork",
			Href:      "/api/hypershell/v1/gateway_networks/" + d.ID,
		},
		Name:         d.Name,
		Topology:     d.Topology,
		TunnelMode:   d.TunnelMode,
		HubGatewayId: d.HubGatewayId,
		Status:       d.Status,
	}
}
