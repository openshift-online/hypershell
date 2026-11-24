package platformGatewayPolicies

import (
	"fmt"
	"sort"

	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/cedar"
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

// settingsToProto maps the singleton for the control planes, including the
// accepted fleet defaults they apply to bundles and sidecars (PLAT-PGP-08). An
// unparsable stored value is an error, never replaced by a guess.
func settingsToProto(s *PolicySettings) (*pb.PlatformPolicySettings, error) {
	out := &pb.PlatformPolicySettings{Mode: s.Mode, DefaultsRefusal: s.DefaultsRefusal}
	if !s.UpdatedAt.IsZero() {
		out.UpdatedAt = timestamppb.New(s.UpdatedAt)
	}
	if d, ok := s.AcceptedDefaults(); ok {
		n, err := cedar.ParseDefaults(d.CPURequest, d.CPULimit, d.MemoryRequest, d.MemoryLimit)
		if err != nil {
			return nil, fmt.Errorf("stored fleet defaults are invalid: %w", err)
		}
		out.Defaults = &pb.PlatformPolicyFleetDefaults{
			CpuRequest:         d.CPURequest,
			CpuLimit:           d.CPULimit,
			MemoryRequest:      d.MemoryRequest,
			MemoryLimit:        d.MemoryLimit,
			CpuRequestMillis:   n.CPURequestMillis,
			CpuLimitMillis:     n.CPULimitMillis,
			MemoryRequestBytes: n.MemoryRequestBytes,
			MemoryLimitBytes:   n.MemoryLimitBytes,
		}
	}
	return out, nil
}
