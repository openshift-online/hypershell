package fleets

import (
	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func fleetToProto(d *Fleet) *pb.Fleet {
	return &pb.Fleet{
		Metadata: &pb.ObjectReference{
			Id:          d.ID,
			CreatedAt:   timestamppb.New(d.CreatedAt),
			UpdatedAt:   timestamppb.New(d.UpdatedAt),
			Kind:        "Fleet",
			Href:        "/api/hypershell/v1/fleets/" + d.ID,
			Traceparent: d.Traceparent,
			Tracestate:  d.Tracestate,
		},
		Name:        d.Name,
		Description: d.Description,
		Status:      d.Status,
	}
}
