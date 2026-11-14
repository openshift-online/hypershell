package policy

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
)

// evaluate runs the enforce-mode policy over a JSON proposed operation.
func evaluate(t *testing.T, proposedJSON string) Decision {
	t.Helper()
	proposed := &structpb.Struct{}
	if proposedJSON != "" {
		if err := protojson.Unmarshal([]byte(proposedJSON), proposed); err != nil {
			t.Fatalf("unmarshalling proposed operation %s: %v", proposedJSON, err)
		}
	}
	return MustNew(ModeEnforce).Evaluate(proposed)
}

// wantPatch is the expected form of one emitted patch.
type wantPatch struct {
	op    string
	path  string
	value string // rendered JSON value
}

func summarizePatches(t *testing.T, decision Decision) []string {
	t.Helper()
	summaries := make([]string, 0, len(decision.Patches))
	for _, patch := range decision.Patches {
		if patch.GetOp() != "add" {
			t.Errorf("patch op = %q, want add", patch.GetOp())
		}
		value := ""
		if patch.GetValue() != nil {
			encoded, err := protojson.Marshal(patch.GetValue())
			if err != nil {
				t.Fatalf("marshalling patch value: %v", err)
			}
			// protojson output carries unstable whitespace; compact it for
			// comparison.
			var compact bytes.Buffer
			if err := json.Compact(&compact, encoded); err != nil {
				t.Fatalf("compacting patch value: %v", err)
			}
			value = compact.String()
		}
		summaries = append(summaries, patch.GetOp()+" "+patch.GetPath()+" "+value)
	}
	return summaries
}

func TestEvaluateFillAndFloor(t *testing.T) {
	cases := []struct {
		name string
		// proposed is the CreateSandbox modify_operation payload as JSON.
		proposed string
		// wantAllowed is the expected decision.
		wantAllowed bool
		// wantOutcome is the expected metric outcome.
		wantOutcome Outcome
		// wantReason matches as a substring when non-empty.
		wantReason string
		// wantDenialReason is the expected machine-readable denial reason.
		wantDenialReason DenialReason
		// wantPatches lists the expected add patches in order.
		wantPatches []wantPatch
	}{
		// ---- Missing requests are filled at the floor ----
		{
			name:        "no resources object at all",
			proposed:    `{"spec": {"template": {"image": "alpine"}}}`,
			wantAllowed: true,
			wantOutcome: OutcomePatched,
			wantPatches: []wantPatch{
				{op: "add", path: "/spec/template/resources", value: `{"cpu":"250m","memory":"1Gi"}`},
			},
		},
		{
			name:        "resources present but requests absent keeps limits",
			proposed:    `{"spec": {"template": {"resources": {"limits": {"cpu": "2", "memory": "4Gi"}}}}}`,
			wantAllowed: true,
			wantOutcome: OutcomePatched,
			wantPatches: []wantPatch{
				{op: "add", path: "/spec/template/resources/requests", value: `{"cpu":"250m","memory":"1Gi"}`},
			},
		},
		{
			name:        "requests present with cpu missing only",
			proposed:    `{"spec": {"template": {"resources": {"requests": {"memory": "2Gi"}}}}}`,
			wantAllowed: true,
			wantOutcome: OutcomePatched,
			wantPatches: []wantPatch{
				{op: "add", path: "/spec/template/resources/requests/cpu", value: `"250m"`},
			},
		},
		{
			name:        "requests present with memory missing only",
			proposed:    `{"spec": {"template": {"resources": {"requests": {"cpu": "500m"}}}}}`,
			wantAllowed: true,
			wantOutcome: OutcomePatched,
			wantPatches: []wantPatch{
				{op: "add", path: "/spec/template/resources/requests/memory", value: `"1Gi"`},
			},
		},
		{
			name:        "both request keys missing emit one patch per key",
			proposed:    `{"spec": {"template": {"resources": {"requests": {}}}}}`,
			wantAllowed: true,
			wantOutcome: OutcomePatched,
			wantPatches: []wantPatch{
				{op: "add", path: "/spec/template/resources/requests/cpu", value: `"250m"`},
				{op: "add", path: "/spec/template/resources/requests/memory", value: `"1Gi"`},
			},
		},
		{
			name:        "null request counts as unset",
			proposed:    `{"spec": {"template": {"resources": {"requests": {"cpu": null, "memory": "1Gi"}}}}}`,
			wantAllowed: true,
			wantOutcome: OutcomePatched,
			wantPatches: []wantPatch{
				{op: "add", path: "/spec/template/resources/requests/cpu", value: `"250m"`},
			},
		},

		// ---- At or above the floor: pass through untouched ----
		{
			name:        "requests at the floor pass",
			proposed:    `{"spec": {"template": {"resources": {"requests": {"cpu": "250m", "memory": "1Gi"}}}}}`,
			wantAllowed: true,
			wantOutcome: OutcomeAllowed,
		},
		{
			name:        "decimal cpu equals milli form",
			proposed:    `{"spec": {"template": {"resources": {"requests": {"cpu": "0.25", "memory": "1Gi"}}}}}`,
			wantAllowed: true,
			wantOutcome: OutcomeAllowed,
		},
		{
			name:        "1024Mi equals 1Gi",
			proposed:    `{"spec": {"template": {"resources": {"requests": {"cpu": "250m", "memory": "1024Mi"}}}}}`,
			wantAllowed: true,
			wantOutcome: OutcomeAllowed,
		},
		{
			name:        "json number cpu equals 250m",
			proposed:    `{"spec": {"template": {"resources": {"requests": {"cpu": 0.25, "memory": 1073741824}}}}}`,
			wantAllowed: true,
			wantOutcome: OutcomeAllowed,
		},
		{
			name:        "large requests pass: no ceiling",
			proposed:    `{"spec": {"template": {"resources": {"requests": {"cpu": "64", "memory": "128Gi"}}}}}`,
			wantAllowed: true,
			wantOutcome: OutcomeAllowed,
		},
		{
			name:        "non-governed request keys pass through",
			proposed:    `{"spec": {"template": {"resources": {"requests": {"cpu": "250m", "memory": "1Gi", "ephemeral-storage": "1Gi", "nvidia.com/gpu": "1"}}}}}`,
			wantAllowed: true,
			wantOutcome: OutcomeAllowed,
		},
		{
			name:        "limits are never checked",
			proposed:    `{"spec": {"template": {"resources": {"requests": {"cpu": "250m", "memory": "1Gi"}, "limits": {"cpu": "10m", "memory": "16Mi"}}}}}`,
			wantAllowed: true,
			wantOutcome: OutcomeAllowed,
		},

		// ---- Below the floor: denied ----
		{
			name:             "cpu below floor",
			proposed:         `{"spec": {"template": {"resources": {"requests": {"cpu": "100m", "memory": "1Gi"}}}}}`,
			wantAllowed:      false,
			wantOutcome:      OutcomeDenied,
			wantReason:       "cpu request 100m is below the 250m floor",
			wantDenialReason: DenialCPUBelowFloor,
		},
		{
			name:             "memory below floor",
			proposed:         `{"spec": {"template": {"resources": {"requests": {"cpu": "1", "memory": "512Mi"}}}}}`,
			wantAllowed:      false,
			wantOutcome:      OutcomeDenied,
			wantReason:       "memory request 512Mi is below the 1Gi floor",
			wantDenialReason: DenialMemoryBelowFloor,
		},
		{
			name:             "partly set with memory below floor",
			proposed:         `{"spec": {"template": {"resources": {"requests": {"memory": "512Mi"}}}}}`,
			wantAllowed:      false,
			wantOutcome:      OutcomeDenied,
			wantReason:       "memory request 512Mi is below the 1Gi floor",
			wantDenialReason: DenialMemoryBelowFloor,
		},
		{
			name:             "1G is below 1Gi",
			proposed:         `{"spec": {"template": {"resources": {"requests": {"cpu": "250m", "memory": "1G"}}}}}`,
			wantAllowed:      false,
			wantOutcome:      OutcomeDenied,
			wantReason:       "memory request 1G is below the 1Gi floor",
			wantDenialReason: DenialMemoryBelowFloor,
		},
		{
			name:             "zero cpu is below floor",
			proposed:         `{"spec": {"template": {"resources": {"requests": {"cpu": "0", "memory": "1Gi"}}}}}`,
			wantAllowed:      false,
			wantOutcome:      OutcomeDenied,
			wantReason:       "cpu request 0 is below the 250m floor",
			wantDenialReason: DenialCPUBelowFloor,
		},
		{
			name:             "json number below floor",
			proposed:         `{"spec": {"template": {"resources": {"requests": {"cpu": 0.1, "memory": "1Gi"}}}}}`,
			wantAllowed:      false,
			wantOutcome:      OutcomeDenied,
			wantReason:       "cpu request 0.1 is below the 250m floor",
			wantDenialReason: DenialCPUBelowFloor,
		},

		// ---- Malformed values on the governed path: denied, not patched ----
		{
			name:             "malformed cpu quantity",
			proposed:         `{"spec": {"template": {"resources": {"requests": {"cpu": "banana", "memory": "1Gi"}}}}}`,
			wantAllowed:      false,
			wantOutcome:      OutcomeDenied,
			wantReason:       `"banana" is not a valid resource quantity`,
			wantDenialReason: DenialCPUInvalidQuantity,
		},
		{
			name:             "empty cpu quantity",
			proposed:         `{"spec": {"template": {"resources": {"requests": {"cpu": "", "memory": "1Gi"}}}}}`,
			wantAllowed:      false,
			wantOutcome:      OutcomeDenied,
			wantReason:       `"" is not a valid resource quantity`,
			wantDenialReason: DenialCPUInvalidQuantity,
		},
		{
			name:             "malformed memory quantity",
			proposed:         `{"spec": {"template": {"resources": {"requests": {"cpu": "250m", "memory": "12ZZ"}}}}}`,
			wantAllowed:      false,
			wantOutcome:      OutcomeDenied,
			wantReason:       `"12ZZ" is not a valid resource quantity`,
			wantDenialReason: DenialMemoryInvalidQuantity,
		},
		{
			name:             "boolean request value",
			proposed:         `{"spec": {"template": {"resources": {"requests": {"cpu": true, "memory": "1Gi"}}}}}`,
			wantAllowed:      false,
			wantOutcome:      OutcomeDenied,
			wantReason:       "must be a quantity string or number, got a boolean",
			wantDenialReason: DenialCPUInvalidQuantity,
		},
		{
			name:             "requests is not an object",
			proposed:         `{"spec": {"template": {"resources": {"requests": "250m"}}}}`,
			wantAllowed:      false,
			wantOutcome:      OutcomeDenied,
			wantReason:       "spec.template.resources.requests must be an object",
			wantDenialReason: DenialRequestsNotObject,
		},
		{
			name:             "resources is not an object",
			proposed:         `{"spec": {"template": {"resources": "big"}}}`,
			wantAllowed:      false,
			wantOutcome:      OutcomeDenied,
			wantReason:       "spec.template.resources must be an object",
			wantDenialReason: DenialResourcesNotObject,
		},

		// ---- Ungoverned shapes pass through to gateway validation ----
		{
			name:        "empty proposed operation",
			proposed:    `{}`,
			wantAllowed: true,
			wantOutcome: OutcomeAllowed,
		},
		{
			name:        "nil proposed operation",
			proposed:    "",
			wantAllowed: true,
			wantOutcome: OutcomeAllowed,
		},
		{
			name:        "no spec: gateway typed validation rejects it",
			proposed:    `{"name": "demo"}`,
			wantAllowed: true,
			wantOutcome: OutcomeAllowed,
		},
		{
			name:        "no template: the gateway inserts a default one, so requests are still filled",
			proposed:    `{"spec": {"command": ["sleep", "1"]}}`,
			wantAllowed: true,
			wantOutcome: OutcomePatched,
			wantPatches: []wantPatch{
				{op: "add", path: "/spec/template", value: `{"resources":{"requests":{"cpu":"250m","memory":"1Gi"}}}`},
			},
		},
		{
			name:        "workload template sandboxes stay ungoverned",
			proposed:    `{"workloadTemplate": "cheap", "spec": {"template": {"resources": {"requests": {"cpu": "10m", "memory": "16Mi"}}}}}`,
			wantAllowed: true,
			wantOutcome: OutcomeAllowed,
		},

		// ---- Driver-config path: kubernetes.containers.agent.resources ----
		{
			name:        "driver agent resources absent are filled",
			proposed:    `{"spec": {"template": {"resources": {"requests": {"cpu": "250m", "memory": "1Gi"}}, "driverConfig": {"kubernetes": {"containers": {"agent": {"image": "agent:1"}}}}}}}`,
			wantAllowed: true,
			wantOutcome: OutcomePatched,
			wantPatches: []wantPatch{
				{op: "add", path: "/spec/template/driverConfig/kubernetes/containers/agent/resources", value: `{"cpu":"250m","memory":"1Gi"}`},
			},
		},
		{
			name:        "driver agent requests absent are filled",
			proposed:    `{"spec": {"template": {"resources": {"requests": {"cpu": "250m", "memory": "1Gi"}}, "driverConfig": {"kubernetes": {"containers": {"agent": {"resources": {"limits": {"cpu": "1"}}}}}}}}}`,
			wantAllowed: true,
			wantOutcome: OutcomePatched,
			wantPatches: []wantPatch{
				{op: "add", path: "/spec/template/driverConfig/kubernetes/containers/agent/resources/requests", value: `{"cpu":"250m","memory":"1Gi"}`},
			},
		},
		{
			name:             "driver agent cpu below floor is denied",
			proposed:         `{"spec": {"template": {"driverConfig": {"kubernetes": {"containers": {"agent": {"resources": {"requests": {"cpu": "100m", "memory": "1Gi"}}}}}}}}}`,
			wantAllowed:      false,
			wantOutcome:      OutcomeDenied,
			wantReason:       "cpu request 100m is below the 250m floor",
			wantDenialReason: DenialCPUBelowFloor,
		},
		{
			name:             "driver agent memory below floor is denied with its path",
			proposed:         `{"spec": {"template": {"driverConfig": {"kubernetes": {"containers": {"agent": {"resources": {"requests": {"cpu": "250m", "memory": "512Mi"}}}}}}}}}`,
			wantAllowed:      false,
			wantOutcome:      OutcomeDenied,
			wantReason:       "spec.template.driverConfig.kubernetes.containers.agent.resources.requests.memory: memory request 512Mi is below the 1Gi floor",
			wantDenialReason: DenialMemoryBelowFloor,
		},
		{
			name:        "driver agent requests partly set",
			proposed:    `{"spec": {"template": {"resources": {"requests": {"cpu": "250m", "memory": "1Gi"}}, "driverConfig": {"kubernetes": {"containers": {"agent": {"resources": {"requests": {"cpu": "500m"}}}}}}}}}`,
			wantAllowed: true,
			wantOutcome: OutcomePatched,
			wantPatches: []wantPatch{
				{op: "add", path: "/spec/template/driverConfig/kubernetes/containers/agent/resources/requests/memory", value: `"1Gi"`},
			},
		},
		{
			name:        "no driver config at all is skipped",
			proposed:    `{"spec": {"template": {"image": "alpine"}}}`,
			wantAllowed: true,
			wantOutcome: OutcomePatched,
			wantPatches: []wantPatch{
				{op: "add", path: "/spec/template/resources", value: `{"cpu":"250m","memory":"1Gi"}`},
			},
		},
		{
			name:        "other driver config keys are not governed",
			proposed:    `{"spec": {"template": {"driverConfig": {"docker": {"userns": "host"}}}}}`,
			wantAllowed: true,
			wantOutcome: OutcomePatched,
			wantPatches: []wantPatch{
				{op: "add", path: "/spec/template/resources", value: `{"cpu":"250m","memory":"1Gi"}`},
			},
		},
		{
			name:        "both governed paths are patched in one evaluation",
			proposed:    `{"spec": {"template": {"resources": {"requests": {"cpu": "1"}}, "driverConfig": {"kubernetes": {"containers": {"agent": {"resources": {"requests": {"memory": "2Gi"}}}}}}}}}`,
			wantAllowed: true,
			wantOutcome: OutcomePatched,
			wantPatches: []wantPatch{
				{op: "add", path: "/spec/template/resources/requests/memory", value: `"1Gi"`},
				{op: "add", path: "/spec/template/driverConfig/kubernetes/containers/agent/resources/requests/cpu", value: `"250m"`},
			},
		},
		{
			name:             "denial on the driver path wins over patching the main path",
			proposed:         `{"spec": {"template": {"driverConfig": {"kubernetes": {"containers": {"agent": {"resources": {"requests": {"cpu": "50m", "memory": "1Gi"}}}}}}}}}`,
			wantAllowed:      false,
			wantOutcome:      OutcomeDenied,
			wantDenialReason: DenialCPUBelowFloor,
		},
		{
			name:             "driver agent resources is not an object",
			proposed:         `{"spec": {"template": {"driverConfig": {"kubernetes": {"containers": {"agent": {"resources": "big"}}}}}}}`,
			wantAllowed:      false,
			wantOutcome:      OutcomeDenied,
			wantReason:       "spec.template.driverConfig.kubernetes.containers.agent.resources must be an object",
			wantDenialReason: DenialResourcesNotObject,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision := evaluate(t, tc.proposed)

			if decision.Allowed != tc.wantAllowed {
				t.Errorf("allowed = %v, want %v (reason: %q)", decision.Allowed, tc.wantAllowed, decision.Reason)
			}
			if decision.Outcome != tc.wantOutcome {
				t.Errorf("outcome = %q, want %q", decision.Outcome, tc.wantOutcome)
			}
			if decision.DenialReason != tc.wantDenialReason {
				t.Errorf("denial reason = %q, want %q", decision.DenialReason, tc.wantDenialReason)
			}
			if tc.wantReason != "" && !strings.Contains(decision.Reason, tc.wantReason) {
				t.Errorf("reason = %q, want it to contain %q", decision.Reason, tc.wantReason)
			}
			if tc.wantAllowed && decision.Reason != "" {
				t.Errorf("allowed decision carries reason %q", decision.Reason)
			}
			if !tc.wantAllowed {
				if decision.StatusCode != StatusCode {
					t.Errorf("status code = %q, want %q", decision.StatusCode, StatusCode)
				}
				if decision.Reason == "" {
					t.Error("denial carries no reason")
				}
				if len(decision.Patches) != 0 {
					t.Errorf("denial carries %d patches, want none", len(decision.Patches))
				}
				return
			}
			if decision.StatusCode != "" {
				t.Errorf("allowed decision carries status code %q", decision.StatusCode)
			}

			got := summarizePatches(t, decision)
			if len(got) != len(tc.wantPatches) {
				t.Fatalf("patches = %v, want %d patches %v", got, len(tc.wantPatches), tc.wantPatches)
			}
			for i, want := range tc.wantPatches {
				if got[i] != want.op+" "+want.path+" "+want.value {
					t.Errorf("patch %d = %q, want %q", i, got[i], want.op+" "+want.path+" "+want.value)
				}
			}
		})
	}
}

// TestOffModeAllowsEverything checks that off mode changes no behaviour:
// below-floor requests pass with no patches.
func TestOffModeAllowsEverything(t *testing.T) {
	pol := MustNew(ModeOff)
	for _, proposedJSON := range []string{
		``,
		`{}`,
		`{"spec": {"template": {"resources": {"requests": {"cpu": "10m", "memory": "16Mi"}}}}}`,
		`{"spec": {"template": {"driverConfig": {"kubernetes": {"containers": {"agent": {"resources": {"requests": {"cpu": "1m"}}}}}}}}}`,
		`{"spec": {"template": {"resources": {"requests": {"cpu": "banana"}}}}}`,
	} {
		proposed := &structpb.Struct{}
		if proposedJSON != "" {
			if err := protojson.Unmarshal([]byte(proposedJSON), proposed); err != nil {
				t.Fatalf("unmarshalling %s: %v", proposedJSON, err)
			}
		}
		decision := pol.Evaluate(proposed)
		if !decision.Allowed {
			t.Errorf("off mode denied %s: %q", proposedJSON, decision.Reason)
		}
		if len(decision.Patches) != 0 {
			t.Errorf("off mode patched %s: %v", proposedJSON, decision.Patches)
		}
		if decision.Outcome != OutcomeAllowed {
			t.Errorf("off mode outcome = %q, want allowed", decision.Outcome)
		}
	}
}

// TestDenialReasonsAreDistinct pins the metrics label values.
func TestDenialReasonsAreDistinct(t *testing.T) {
	reasons := []DenialReason{
		DenialCPUBelowFloor, DenialMemoryBelowFloor,
		DenialCPUInvalidQuantity, DenialMemoryInvalidQuantity,
		DenialResourcesNotObject, DenialRequestsNotObject,
	}
	seen := map[DenialReason]bool{}
	for _, reason := range reasons {
		if reason == "" {
			t.Errorf("empty denial reason in %v", reasons)
		}
		if seen[reason] {
			t.Errorf("duplicate denial reason %q", reason)
		}
		seen[reason] = true
	}
}

// TestPatchValueShapes checks the emitted patch values are well-formed
// structpb values the gateway can apply.
func TestPatchValueShapes(t *testing.T) {
	decision := evaluate(t, `{"spec": {"template": {"resources": {}}}}`)
	if len(decision.Patches) != 1 {
		t.Fatalf("patches = %d, want 1", len(decision.Patches))
	}
	value := decision.Patches[0].GetValue()
	if value.GetStructValue().GetFields()["cpu"].GetStringValue() != CPUFloor {
		t.Errorf("patch cpu = %v, want %q", value, CPUFloor)
	}
	if value.GetStructValue().GetFields()["memory"].GetStringValue() != MemoryFloor {
		t.Errorf("patch memory = %v, want %q", value, MemoryFloor)
	}
}
