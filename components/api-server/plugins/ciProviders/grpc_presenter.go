package ciProviders

import (
	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func ciProviderToProto(p *CIProvider) *pb.CIProvider {
	return &pb.CIProvider{
		Metadata: &pb.ObjectReference{
			Id:        p.ID,
			CreatedAt: timestamppb.New(p.CreatedAt),
			UpdatedAt: timestamppb.New(p.UpdatedAt),
			Kind:      "CIProvider",
			Href:      "/api/hypershell/ext/ci_providers/" + p.ID,
		},
		Forge:          p.Forge,
		Issuer:         p.Issuer,
		JwksUrl:        p.JwksUrl,
		Audience:       p.Audience,
		StatusIdpAlias: p.StatusIdpAlias,
	}
}
