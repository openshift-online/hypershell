package gatewayReleases

import (
	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func gatewayReleaseToProto(d *GatewayRelease) *pb.GatewayRelease {
	return &pb.GatewayRelease{
		Metadata: &pb.ObjectReference{
			Id:          d.ID,
			CreatedAt:   timestamppb.New(d.CreatedAt),
			UpdatedAt:   timestamppb.New(d.UpdatedAt),
			Kind:        "GatewayRelease",
			Href:        "/api/hypershell/v1/gateway_releases/" + d.ID,
			Traceparent: d.Traceparent,
			Tracestate:  d.Tracestate,
		},
		Name:            d.Name,
		FleetId:         d.FleetId,
		Image:           d.Image,
		RolloutStrategy: d.RolloutStrategy,
		CanaryPercent: func() *int32 {
			if d.CanaryPercent != nil {
				v := int32(*d.CanaryPercent)
				return &v
			}
			return nil
		}(),
		CanaryDuration: d.CanaryDuration,
		Status:         d.Status,
	}
}
