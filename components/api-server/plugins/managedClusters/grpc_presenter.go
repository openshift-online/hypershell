package managedClusters

import (
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func managedClusterToProto(d *ManagedCluster) *pb.ManagedCluster {
	return &pb.ManagedCluster{
		Metadata: &pb.ObjectReference{
			Id:          d.ID,
			CreatedAt:   timestamppb.New(d.CreatedAt),
			UpdatedAt:   timestamppb.New(d.UpdatedAt),
			Kind:        "ManagedCluster",
			Href:        "/api/hypershell/v1/managed_clusters/" + d.ID,
			Traceparent: d.Traceparent,
			Tracestate:  d.Tracestate,
		},
		Name:             d.Name,
		KubeconfigSecret: d.KubeconfigSecret,
		Status:           d.Status,
		ApiServerUrl:     d.ApiServerUrl,
		Capabilities:     d.Capabilities,
		Health:           healthVerdictToProto(d.Health),
	}
}

// healthVerdictToProto surfaces the stored self-check verdict over gRPC reads
// (PLAT-DATA-27). A nil verdict (never reported) stays nil.
func healthVerdictToProto(v *HealthVerdict) *pb.ManagedClusterHealth {
	if v == nil {
		return nil
	}
	health := &pb.ManagedClusterHealth{
		Overall:    v.Overall,
		DurationMs: v.DurationMs,
	}
	if v.ObservedAt != nil {
		observedAt := v.ObservedAt.Format(time.RFC3339Nano)
		health.ObservedAt = &observedAt
	}
	health.Conditions = make([]*pb.ManagedClusterHealthCondition, 0, len(v.Conditions))
	for i := range v.Conditions {
		c := v.Conditions[i]
		condition := &pb.ManagedClusterHealthCondition{
			Name:    c.Name,
			Status:  c.Status,
			Message: c.Message,
		}
		if c.LastTransitionAt != nil {
			lastTransitionAt := c.LastTransitionAt.Format(time.RFC3339Nano)
			condition.LastTransitionAt = &lastTransitionAt
		}
		health.Conditions = append(health.Conditions, condition)
	}
	return health
}
