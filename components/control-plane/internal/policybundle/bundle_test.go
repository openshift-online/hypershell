package policybundle

import (
	"strings"
	"testing"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/cedar"
)

func policy(id, name, mode, cedarText string, target *pb.PlatformGatewayPolicyTarget) *pb.PlatformGatewayPolicy {
	return &pb.PlatformGatewayPolicy{
		Metadata:        &pb.ObjectReference{Id: id},
		Name:            name,
		EnforcementMode: mode,
		Cedar:           cedarText,
		UserMessage:     name + " message",
		Target:          target,
	}
}

const cpuMax = `forbid (principal, action, resource) when { context.cpuRequestMillis > 4000 };`

var defaults = cedar.Defaults{CPURequestMillis: 500, CPULimitMillis: 2000, MemoryRequestBytes: 1 << 30, MemoryLimitBytes: 4 << 30}

// Verifies: CTRL-CORE-22.01, PLAT-PGP-06.01
func TestBuildResolvesEffectiveModesAndTargets(t *testing.T) {
	all := &pb.PlatformGatewayPolicyTarget{All: true}
	policies := []*pb.PlatformGatewayPolicy{
		policy("1", "b-audit", ModeAudit, cpuMax, all),
		policy("2", "a-enforce", ModeEnforce, cpuMax, all),
		policy("3", "excluded", ModeEnforce, cpuMax, &pb.PlatformGatewayPolicyTarget{All: true, Exclude: []string{"gw-1"}}),
		policy("4", "selected-other", ModeEnforce, cpuMax, &pb.PlatformGatewayPolicyTarget{GatewayIds: []string{"gw-2"}}),
	}
	b, skipped, err := Build(Input{GatewayID: "gw-1", GlobalMode: ModeAudit, Policies: policies, Defaults: defaults, CanaryEnabled: true})
	if err != nil || len(skipped) != 0 {
		t.Fatalf("Build: %v skipped=%v", err, skipped)
	}
	if len(b.Enforced) != 1 || b.Enforced[0].Name != "a-enforce" || len(b.AuditOnly) != 1 || b.AuditOnly[0].Name != "b-audit" {
		t.Fatalf("unexpected sets: %+v", b)
	}
	if !strings.Contains(b.Enforced[0].Cedar, `@id("a-enforce")`) {
		t.Fatalf("statements must carry @id: %s", b.Enforced[0].Cedar)
	}
	b2, _, err := Build(Input{GatewayID: "gw-1", GlobalMode: ModeEnforce, Policies: policies, Defaults: defaults, CanaryEnabled: true})
	if err != nil || len(b2.Enforced) != 2 || len(b2.AuditOnly) != 0 {
		t.Fatalf("global enforce must enforce every targeted policy: %+v %v", b2, err)
	}
	if b.Revision == b2.Revision {
		t.Fatalf("a mode change must change the revision")
	}
}

// Verifies: STD-RCON-02, PLAT-PGP-12.01
func TestBuildIsDeterministicAndDecodes(t *testing.T) {
	policies := []*pb.PlatformGatewayPolicy{policy("2", "z", ModeEnforce, cpuMax, nil), policy("1", "a", ModeAudit, cpuMax, nil)}
	reversed := []*pb.PlatformGatewayPolicy{policies[1], policies[0]}
	b1, _, _ := Build(Input{GatewayID: "gw", GlobalMode: ModeAudit, Policies: policies, Defaults: defaults})
	b2, _, _ := Build(Input{GatewayID: "gw", GlobalMode: ModeAudit, Policies: reversed, Defaults: defaults})
	if b1.Revision != b2.Revision {
		t.Fatalf("input order must not change the revision")
	}
	raw, err := b1.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(raw)
	if err != nil || got.Revision != b1.Revision {
		t.Fatalf("Decode: %v", err)
	}
	tampered := strings.Replace(string(raw), "4000", "9000", 1)
	if _, err := Decode([]byte(tampered)); err == nil {
		t.Fatalf("a bundle whose content does not match its revision must be rejected")
	}
	wrongSchema := strings.Replace(string(raw), `"schemaVersion": "v1"`, `"schemaVersion": "v9"`, 1)
	if _, err := Decode([]byte(wrongSchema)); err == nil {
		t.Fatalf("an unknown schema version must be reported, not evaluated (PLAT-PGP-03.02)")
	}
}

// Verifies: PLAT-PGP-04, PLAT-PGP-18.01, PLAT-PGP-20.02
func TestBuildSkipsInvalidDisabledAndUnsupported(t *testing.T) {
	canary := policy("c", CanaryPolicyName, ModeEnforce, `forbid (principal, action, resource) when { context.sandboxName like "hypershell-e2e-policy-*" && context.cpuRequestMillis > 1000 };`, nil)
	canary.BuiltIn = true
	forbidsDefaults := policy("d", "too-strict", ModeEnforce, `forbid (principal, action, resource) when { context.cpuRequestMillis < 600 };`, nil)
	b, skipped, err := Build(Input{GatewayID: "gw", GlobalMode: ModeAudit, Policies: []*pb.PlatformGatewayPolicy{canary, forbidsDefaults}, Defaults: defaults, CanaryEnabled: false})
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Enforced)+len(b.AuditOnly) != 0 || len(skipped) != 2 {
		t.Fatalf("disabled canary and re-validation failure must be skipped: %+v %+v", b, skipped)
	}
	b, _, _ = Build(Input{GatewayID: "gw", GlobalMode: ModeEnforce, Policies: []*pb.PlatformGatewayPolicy{canary}, Defaults: defaults, CanaryEnabled: true, Unsupported: true})
	if len(b.Enforced) != 0 || len(b.AuditOnly) != 1 || b.ResourcePoliciesSupported {
		t.Fatalf("an unsupported gateway must never enforce: %+v", b)
	}
}
