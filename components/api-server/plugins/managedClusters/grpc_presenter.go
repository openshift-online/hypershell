package managedClusters

import (
	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func managedClusterToProto(d *ManagedCluster) *pb.ManagedCluster {
	return &pb.ManagedCluster{
		Metadata: &pb.ObjectReference{
			Id:        d.ID,
			CreatedAt: timestamppb.New(d.CreatedAt),
			UpdatedAt: timestamppb.New(d.UpdatedAt),
			Kind:      "ManagedCluster",
			Href:      "/api/hypershell/v1/managed_clusters/" + d.ID,
		},
		Name:             d.Name,
		Provider:         d.Provider,
		Region:           d.Region,
		KubeconfigSecret: d.KubeconfigSecret,
		Status:           d.Status,
		ApiServerUrl:     d.ApiServerUrl,
		ProfileId:        d.ProfileId,
		DatabaseId:       d.DatabaseId,
	}
}
