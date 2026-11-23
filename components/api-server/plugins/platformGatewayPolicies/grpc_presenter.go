package platformGatewayPolicies

import (
	"sort"

	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
)

func policyToProto(p *PlatformGatewayPolicy) (*pb.PlatformGatewayPolicy, error) {
	target, err := DecodeTarget(p.Target)
	if err != nil {
		return nil, err
	}
	st, err := DecodeStatus(p.Status)
	if err != nil {
		return nil, err
	}
	out := &pb.PlatformGatewayPolicy{
		Metadata: &pb.ObjectReference{
			Id:        p.ID,
			CreatedAt: timestamppb.New(p.CreatedAt),
			UpdatedAt: timestamppb.New(p.UpdatedAt),
			Kind:      "PlatformGatewayPolicy",
			Href:      "/api/hypershell/ext/platform_gateway_policies/" + p.ID,
		},
		Name:             p.Name,
		Description:      p.Description,
		Target:           &pb.PlatformGatewayPolicyTarget{All: target.All, GatewayIds: target.GatewayIDs, Exclude: target.Exclude},
		EnforcementMode:  p.EnforcementMode,
		UserMessage:      p.UserMessage,
		DocumentationUrl: p.DocumentationUrl,
		Cedar:            p.Cedar,
		BuiltIn:          p.BuiltIn,
	}
	for _, g := range sortedGatewayStatuses(st) {
		entry := &pb.PlatformGatewayPolicyGatewayStatus{
			GatewayId:             g.GatewayID,
			Condition:             g.Condition,
			Revision:              g.Revision,
			AppliedRevision:       g.AppliedRevision,
			EffectiveMode:         g.EffectiveMode,
			Message:               g.Message,
			NonCompliantSandboxes: g.NonCompliantSandboxes,
		}
		if g.ObservedAt != nil {
			entry.ObservedAt = timestamppb.New(*g.ObservedAt)
		}
		out.GatewayStatuses = append(out.GatewayStatuses, entry)
	}
	return out, nil
}

func sortedGatewayStatuses(st StoredStatus) []GatewayStatus {
	out := make([]GatewayStatus, 0, len(st.Gateways))
	for _, g := range st.Gateways {
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].GatewayID < out[j].GatewayID })
	return out
}

func settingsToProto(s *PolicySettings) *pb.PlatformPolicySettings {
	out := &pb.PlatformPolicySettings{Mode: s.Mode}
	if !s.UpdatedAt.IsZero() {
		out.UpdatedAt = timestamppb.New(s.UpdatedAt)
	}
	return out
}
