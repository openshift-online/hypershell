package managedDatabases

import (
	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func managedDatabaseToProto(d *ManagedDatabase) *pb.ManagedDatabase {
	return &pb.ManagedDatabase{
		Metadata: &pb.ObjectReference{
			Id:        d.ID,
			CreatedAt: timestamppb.New(d.CreatedAt),
			UpdatedAt: timestamppb.New(d.UpdatedAt),
			Kind:      "ManagedDatabase",
			Href:      "/api/hypershell/v1/managed_databases/" + d.ID,
		},
		Name:             d.Name,
		FleetId:          d.FleetId,
		Provider:         d.Provider,
		Namespace:        d.Namespace,
		Region:           d.Region,
		Engine:           d.Engine,
		EngineVersion:    d.EngineVersion,
		InstanceClass:    d.InstanceClass,
		ConnectionSecret: d.ConnectionSecret,
		Status:           d.Status,
	}
}
