package platformGatewayPolicies

import (
	"sort"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/pkg/cedar"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api/presenters"
)

func presentTarget(t Target) *openapi.PlatformGatewayPolicyTarget {
	out := &openapi.PlatformGatewayPolicyTarget{All: openapi.PtrBool(t.All)}
	if len(t.GatewayIDs) > 0 {
		out.GatewayIds = append([]string{}, t.GatewayIDs...)
	}
	if len(t.Exclude) > 0 {
		out.Exclude = append([]string{}, t.Exclude...)
	}
	return out
}

// convertTarget maps the API target to the model. An absent target or one with
// neither all nor gateway ids means all gateways.
func convertTarget(t *openapi.PlatformGatewayPolicyTarget) Target {
	if t == nil {
		return Target{All: true}
	}
	out := Target{GatewayIDs: t.GatewayIds, Exclude: t.Exclude}
	if t.All != nil {
		out.All = *t.All
	} else {
		out.All = len(t.GatewayIds) == 0
	}
	return out
}

func presentRendered(cedarText string) []openapi.PolicyRenderedRule {
	rules, err := cedar.RenderPolicy(cedarText)
	if err != nil {
		return nil
	}
	out := make([]openapi.PolicyRenderedRule, 0, len(rules))
	for _, r := range rules {
		item := openapi.PolicyRenderedRule{
			Recognized: openapi.PtrBool(r.Recognized),
			Cedar:      openapi.PtrString(r.Cedar),
		}
		if len(r.Summary) > 0 {
			item.Summary = append([]string{}, r.Summary...)
		}
		for _, b := range r.Bounds {
			item.Bounds = append(item.Bounds, openapi.PolicyBound{
				Field:   openapi.PtrString(b.Field),
				Label:   openapi.PtrString(b.Label),
				Op:      openapi.PtrString(b.Op),
				Value:   openapi.PtrInt64(b.Value),
				Display: openapi.PtrString(b.Display),
				Text:    openapi.PtrString(b.Text),
			})
		}
		out = append(out, item)
	}
	return out
}

func presentDiagnostics(ve *cedar.ValidationError) []openapi.PolicyDiagnostic {
	if ve == nil {
		return nil
	}
	out := make([]openapi.PolicyDiagnostic, 0, len(ve.Details))
	for _, d := range ve.Details {
		item := openapi.PolicyDiagnostic{
			Line:      openapi.PtrInt32(int32(d.Line)),
			Statement: openapi.PtrInt32(int32(d.Statement)),
			Message:   openapi.PtrString(d.Message),
		}
		if d.Suggestion != "" {
			item.Suggestion = openapi.PtrString(d.Suggestion)
		}
		out = append(out, item)
	}
	return out
}

// PresentFleetDefaults maps the configured defaults to the API shape.
func PresentFleetDefaults(d FleetDefaults) *openapi.FleetDefaults {
	return &openapi.FleetDefaults{
		CpuRequest:         openapi.PtrString(d.CPURequest),
		CpuLimit:           openapi.PtrString(d.CPULimit),
		MemoryRequest:      openapi.PtrString(d.MemoryRequest),
		MemoryLimit:        openapi.PtrString(d.MemoryLimit),
		CpuRequestMillis:   openapi.PtrInt64(d.Normalized.CPURequestMillis),
		CpuLimitMillis:     openapi.PtrInt64(d.Normalized.CPULimitMillis),
		MemoryRequestBytes: openapi.PtrInt64(d.Normalized.MemoryRequestBytes),
		MemoryLimitBytes:   openapi.PtrInt64(d.Normalized.MemoryLimitBytes),
	}
}

func presentCounts(c FindingCounts) *openapi.PolicyFindingCounts {
	return &openapi.PolicyFindingCounts{
		WouldReject:      openapi.PtrInt32(c.WouldReject),
		WouldAddDefaults: openapi.PtrInt32(c.WouldAddDefaults),
	}
}

// policyView carries the read-time facts a presented policy needs.
type policyView struct {
	globalMode      string
	validation      *cedar.ValidationError
	counts24h       FindingCounts
	gatewayCounts   map[string]int32
	includeTarget   bool
	includeRollouts bool
}

// PresentPolicy maps a stored policy to the API shape with its effective mode,
// rendered rules, validation result, per-gateway rollout, and 24-hour counts.
func PresentPolicy(p *PlatformGatewayPolicy, v policyView) openapi.PlatformGatewayPolicy {
	reference := presenters.PresentReference(p.ID, p)
	out := openapi.PlatformGatewayPolicy{
		Id:               reference.Id,
		Kind:             reference.Kind,
		Href:             reference.Href,
		CreatedAt:        openapi.PtrTime(p.CreatedAt),
		UpdatedAt:        openapi.PtrTime(p.UpdatedAt),
		Name:             p.Name,
		Description:      openapi.PtrString(p.Description),
		EnforcementMode:  openapi.PtrString(p.EnforcementMode),
		EffectiveMode:    openapi.PtrString(EffectiveMode(v.globalMode, p.EnforcementMode)),
		UserMessage:      openapi.PtrString(p.UserMessage),
		DocumentationUrl: openapi.PtrString(p.DocumentationUrl),
		Cedar:            p.Cedar,
		BuiltIn:          openapi.PtrBool(p.BuiltIn),
		Rendered:         presentRendered(p.Cedar),
	}
	if v.includeTarget {
		if t, err := DecodeTarget(p.Target); err == nil {
			out.Target = presentTarget(t)
		}
	}
	status := &openapi.PlatformGatewayPolicyStatus{
		Validation: &openapi.PlatformGatewayPolicyValidation{
			Valid:         openapi.PtrBool(v.validation == nil),
			SchemaVersion: openapi.PtrString(cedar.SchemaVersion),
			Diagnostics:   presentDiagnostics(v.validation),
		},
		FindingsLast24h: presentCounts(v.counts24h),
	}
	if v.includeRollouts {
		st, err := DecodeStatus(p.Status)
		if err == nil {
			var applied, pending, unsupported int32
			ids := make([]string, 0, len(st.Gateways))
			for id := range st.Gateways {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			for _, id := range ids {
				g := st.Gateways[id]
				switch g.Condition {
				case ConditionApplied:
					applied++
				case ConditionPending:
					pending++
				case ConditionUnsupported:
					unsupported++
				}
				status.Gateways = append(status.Gateways, presentGatewayStatus(g, v.gatewayCounts[id]))
			}
			status.Rollout = &openapi.PlatformGatewayPolicyStatusRollout{
				Applied:     openapi.PtrInt32(applied),
				Pending:     openapi.PtrInt32(pending),
				Unsupported: openapi.PtrInt32(unsupported),
			}
		}
	}
	out.Status = status
	return out
}

func presentGatewayStatus(g GatewayStatus, wouldReject int32) openapi.PlatformGatewayPolicyGatewayStatus {
	out := openapi.PlatformGatewayPolicyGatewayStatus{
		GatewayId:      openapi.PtrString(g.GatewayID),
		Condition:      openapi.PtrString(g.Condition),
		WouldReject24h: openapi.PtrInt32(wouldReject),
	}
	if g.Revision != "" {
		out.Revision = openapi.PtrString(g.Revision)
	}
	if g.AppliedRevision != "" {
		out.AppliedRevision = openapi.PtrString(g.AppliedRevision)
	}
	if g.EffectiveMode != "" {
		out.EffectiveMode = openapi.PtrString(g.EffectiveMode)
	}
	if g.Message != "" {
		out.Message = openapi.PtrString(g.Message)
	}
	if len(g.NonCompliantSandboxes) > 0 {
		out.NonCompliantSandboxes = append([]string{}, g.NonCompliantSandboxes...)
	}
	if g.ObservedAt != nil {
		out.ObservedAt = openapi.PtrTime(*g.ObservedAt)
	}
	return out
}

// PresentFinding maps a stored finding to the API shape.
func PresentFinding(f Finding) openapi.PolicyAuditFinding {
	out := openapi.PolicyAuditFinding{
		Id:               openapi.PtrString(f.ID),
		OccurredAt:       openapi.PtrTime(f.OccurredAt),
		GatewayId:        openapi.PtrString(f.GatewayID),
		PolicyNames:      append([]string{}, f.PolicyNames...),
		BundleRevision:   openapi.PtrString(f.BundleRevision),
		PrincipalSubject: openapi.PtrString(f.PrincipalSubject),
		Operation:        openapi.PtrString(f.Operation),
		Outcome:          openapi.PtrString(f.Outcome),
	}
	if f.RequestSummary != nil {
		summary := make(map[string]int64, len(f.RequestSummary))
		for k, v := range f.RequestSummary {
			summary[k] = v
		}
		out.RequestSummary = &summary
	}
	return out
}

// PresentSettings maps the settings singleton and the fleet defaults.
func PresentSettings(s *PolicySettings, d FleetDefaults) openapi.PlatformPolicySettings {
	out := openapi.PlatformPolicySettings{
		Kind:          openapi.PtrString("PlatformPolicySettings"),
		Mode:          openapi.PtrString(s.Mode),
		SchemaVersion: openapi.PtrString(cedar.SchemaVersion),
		Defaults:      PresentFleetDefaults(d),
	}
	if !s.UpdatedAt.IsZero() {
		out.UpdatedAt = openapi.PtrTime(s.UpdatedAt)
	}
	if s.UpdatedBy != "" {
		out.UpdatedBy = openapi.PtrString(s.UpdatedBy)
	}
	return out
}
