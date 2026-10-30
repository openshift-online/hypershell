package ciTrusts

import (
	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func ciTrustToProto(t *CITrust) *pb.CITrust {
	return &pb.CITrust{
		Metadata: &pb.ObjectReference{
			Id:        t.ID,
			CreatedAt: timestamppb.New(t.CreatedAt),
			UpdatedAt: timestamppb.New(t.UpdatedAt),
			Kind:      "CITrust",
			Href:      "/api/hypershell/ext/ci_trusts/" + t.ID,
		},
		CiProviderId:           t.CIProviderID,
		GatewayId:              t.GatewayID,
		SubjectRepository:      t.SubjectRepository,
		SubjectProjectPath:     t.SubjectProjectPath,
		SubjectRef:             t.SubjectRef,
		SubjectEnvironment:     t.SubjectEnvironment,
		SubjectRefProtected:    t.SubjectRefProtected,
		Role:                   t.Role,
		StatusExchangeEndpoint: t.StatusExchangeEndpoint,
		StatusWorkspace:        t.StatusWorkspace,
		StatusWorkflowHint:     t.StatusWorkflowHint,
	}
}
