package server

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/openshift-online/hypershell/components/sandbox-resource-interceptor/internal/policy"
	pb "github.com/openshift-online/hypershell/components/sandbox-resource-interceptor/pkg/api/grpc"
)

func gatewayMetadata() *pb.PeerMetadata {
	return &pb.PeerMetadata{
		ProtocolVersion:       &pb.ProtocolVersion{Major: 1, Minor: 0},
		ImplementationName:    "openshell-gateway",
		ImplementationVersion: "v0.1.8-rhaiv.0",
		SupportedCapabilities: []string{ContractCapability},
		RequiredCapabilities:  []string{ContractCapability},
	}
}

// fakeRecorder records evaluation outcomes without Prometheus.
type fakeRecorder struct {
	calls   int
	outcome policy.Outcome
	reason  policy.DenialReason
}

func (f *fakeRecorder) RecordEvaluation(outcome policy.Outcome, reason policy.DenialReason) {
	f.calls++
	f.outcome = outcome
	f.reason = reason
}

// newServer builds an enforce-mode server with a recording fake.
func newServer(mode policy.Mode) (*Server, *fakeRecorder) {
	recorder := &fakeRecorder{}
	return New("test-version", policy.MustNew(mode), recorder), recorder
}

// evaluation builds a modify_operation evaluation over a JSON proposed
// operation.
func evaluation(t *testing.T, proposedJSON string) *pb.InterceptorEvaluation {
	t.Helper()
	proposed := &structpb.Struct{}
	if proposedJSON != "" {
		if err := protojson.Unmarshal([]byte(proposedJSON), proposed); err != nil {
			t.Fatalf("unmarshalling proposed operation %s: %v", proposedJSON, err)
		}
	}
	return &pb.InterceptorEvaluation{
		InterceptorName: Name,
		BindingId:       CreateSandboxBindingID,
		Service:         "openshell.v1.OpenShell",
		Method:          "CreateSandbox",
		Phase: &pb.InterceptorEvaluation_ModifyOperation{
			ModifyOperation: &pb.ModifyOperationEvaluation{ProposedOperation: proposed},
		},
	}
}

func TestDescribeManifest(t *testing.T) {
	srv, _ := newServer(policy.ModeEnforce)
	manifest, err := srv.Describe(context.Background(), &pb.DescribeRequest{Gateway: gatewayMetadata()})
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if manifest.GetName() != Name {
		t.Errorf("name = %q, want %q", manifest.GetName(), Name)
	}
	if manifest.GetExpectedAudience() != ExpectedAudience {
		t.Errorf("expected_audience = %q, want %q", manifest.GetExpectedAudience(), ExpectedAudience)
	}
	if manifest.GetFailurePolicy() != "fail_closed" {
		t.Errorf("failure_policy = %q, want fail_closed", manifest.GetFailurePolicy())
	}
	if manifest.GetProviderProfiles() {
		t.Error("provider_profiles = true, want false")
	}

	if len(manifest.GetBindings()) != 1 {
		t.Fatalf("bindings = %d, want 1", len(manifest.GetBindings()))
	}
	binding := manifest.GetBindings()[0]
	if binding.GetId() != CreateSandboxBindingID {
		t.Errorf("binding id = %q", binding.GetId())
	}
	if binding.GetSelector().GetRpc() != CreateSandboxRPC {
		t.Errorf("binding rpc = %q, want %q", binding.GetSelector().GetRpc(), CreateSandboxRPC)
	}
	if binding.GetFailurePolicy() != "fail_closed" {
		t.Errorf("binding failure_policy = %q, want fail_closed", binding.GetFailurePolicy())
	}
	phases := binding.GetPhases()
	if len(phases) != 1 || phases[0] != pb.GatewayInterceptorPhase_GATEWAY_INTERCEPTOR_PHASE_MODIFY_OPERATION {
		t.Errorf("binding phases = %v, want [MODIFY_OPERATION]", phases)
	}

	ext := manifest.GetExtension()
	if ext.GetProtocolVersion().GetMajor() != ProtocolMajor || ext.GetProtocolVersion().GetMinor() != ProtocolMinor {
		t.Errorf("extension protocol = %v", ext.GetProtocolVersion())
	}
	if !hasCapability(ext.GetSupportedCapabilities(), ContractCapability) {
		t.Errorf("extension supported_capabilities = %v, missing %s", ext.GetSupportedCapabilities(), ContractCapability)
	}
	if ext.GetImplementationName() != ImplementationName {
		t.Errorf("implementation_name = %q", ext.GetImplementationName())
	}
	if ext.GetImplementationVersion() != "test-version" {
		t.Errorf("implementation_version = %q", ext.GetImplementationVersion())
	}
}

func TestDescribeNegotiation(t *testing.T) {
	srv, _ := newServer(policy.ModeEnforce)

	cases := []struct {
		name    string
		gateway *pb.PeerMetadata
	}{
		{name: "no metadata", gateway: nil},
		{
			name: "no protocol version",
			gateway: &pb.PeerMetadata{
				SupportedCapabilities: []string{ContractCapability},
			},
		},
		{
			name: "unsupported protocol major",
			gateway: &pb.PeerMetadata{
				ProtocolVersion:       &pb.ProtocolVersion{Major: 2, Minor: 0},
				SupportedCapabilities: []string{ContractCapability},
			},
		},
		{
			name: "missing contract capability",
			gateway: &pb.PeerMetadata{
				ProtocolVersion:       &pb.ProtocolVersion{Major: 1, Minor: 0},
				SupportedCapabilities: []string{"something.else"},
			},
		},
		{
			name: "unsupported gateway requirement",
			gateway: &pb.PeerMetadata{
				ProtocolVersion:       &pb.ProtocolVersion{Major: 1, Minor: 0},
				SupportedCapabilities: []string{ContractCapability},
				RequiredCapabilities:  []string{ContractCapability, "openshell.unknown.contract"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := srv.Describe(context.Background(), &pb.DescribeRequest{Gateway: tc.gateway})
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("Describe code = %v, want InvalidArgument (err: %v)", status.Code(err), err)
			}
		})
	}

	t.Run("newer minor protocol accepted", func(t *testing.T) {
		gateway := gatewayMetadata()
		gateway.ProtocolVersion.Minor = 5
		if _, err := srv.Describe(context.Background(), &pb.DescribeRequest{Gateway: gateway}); err != nil {
			t.Fatalf("Describe with protocol 1.5: %v", err)
		}
	})
}

func TestEvaluateEnforce(t *testing.T) {
	t.Run("fills missing requests", func(t *testing.T) {
		srv, recorder := newServer(policy.ModeEnforce)
		result, err := srv.Evaluate(context.Background(), evaluation(t,
			`{"spec": {"template": {"image": "alpine"}}}`))
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		if !result.GetAllowed() {
			t.Fatalf("allowed = false, reason %q", result.GetReason())
		}
		if len(result.GetPatches()) != 1 || result.GetPatches()[0].GetPath() != "/spec/template/resources" {
			t.Fatalf("patches = %v, want one add of /spec/template/resources", result.GetPatches())
		}
		if recorder.calls != 1 || recorder.outcome != policy.OutcomePatched || recorder.reason != "" {
			t.Errorf("recorder = %d/%q/%q, want one patched evaluation with empty reason",
				recorder.calls, recorder.outcome, recorder.reason)
		}
	})

	t.Run("denies below the floor", func(t *testing.T) {
		srv, recorder := newServer(policy.ModeEnforce)
		result, err := srv.Evaluate(context.Background(), evaluation(t,
			`{"spec": {"template": {"resources": {"requests": {"cpu": "100m", "memory": "1Gi"}}}}}`))
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		if result.GetAllowed() {
			t.Fatal("allowed = true, want denied")
		}
		if result.GetStatusCode() != "FAILED_PRECONDITION" {
			t.Errorf("status_code = %q, want FAILED_PRECONDITION", result.GetStatusCode())
		}
		if result.GetReason() == "" {
			t.Error("denial carries no reason")
		}
		if len(result.GetPatches()) != 0 {
			t.Errorf("denial carries patches %v, want none", result.GetPatches())
		}
		if recorder.calls != 1 || recorder.outcome != policy.OutcomeDenied || recorder.reason != policy.DenialCPUBelowFloor {
			t.Errorf("recorder = %d/%q/%q, want one denied cpu_below_floor evaluation",
				recorder.calls, recorder.outcome, recorder.reason)
		}
	})

	t.Run("allows requests at the floor with no patches", func(t *testing.T) {
		srv, recorder := newServer(policy.ModeEnforce)
		result, err := srv.Evaluate(context.Background(), evaluation(t,
			`{"spec": {"template": {"resources": {"requests": {"cpu": "250m", "memory": "1Gi"}}}}}`))
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		if !result.GetAllowed() || len(result.GetPatches()) != 0 {
			t.Fatalf("result = %v, want allowed with no patches", result)
		}
		if recorder.calls != 1 || recorder.outcome != policy.OutcomeAllowed || recorder.reason != "" {
			t.Errorf("recorder = %d/%q/%q, want one allowed evaluation with empty reason",
				recorder.calls, recorder.outcome, recorder.reason)
		}
	})
}

// TestEvaluateOffModeAllowsEverything checks that off mode leaves today's
// behaviour unchanged even for below-floor requests.
func TestEvaluateOffModeAllowsEverything(t *testing.T) {
	srv, recorder := newServer(policy.ModeOff)
	result, err := srv.Evaluate(context.Background(), evaluation(t,
		`{"spec": {"template": {"resources": {"requests": {"cpu": "100m", "memory": "512Mi"}}}}}`))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !result.GetAllowed() {
		t.Fatalf("allowed = false, reason %q", result.GetReason())
	}
	if len(result.GetPatches()) != 0 {
		t.Errorf("patches = %v, want none in off mode", result.GetPatches())
	}
	if recorder.calls != 1 || recorder.outcome != policy.OutcomeAllowed || recorder.reason != "" {
		t.Errorf("recorder = %d/%q/%q, want one allowed evaluation with empty reason",
			recorder.calls, recorder.outcome, recorder.reason)
	}
}

// A nil recorder must not break evaluation.
func TestEvaluateWithoutRecorder(t *testing.T) {
	srv := New("test-version", policy.MustNew(policy.ModeEnforce), nil)
	result, err := srv.Evaluate(context.Background(), evaluation(t,
		`{"spec": {"template": {"image": "alpine"}}}`))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !result.GetAllowed() || len(result.GetPatches()) != 1 {
		t.Fatalf("result = %v, want allowed with one patch", result)
	}
}

func TestEvaluateRejectsMissingPhase(t *testing.T) {
	srv, _ := newServer(policy.ModeEnforce)
	_, err := srv.Evaluate(context.Background(), &pb.InterceptorEvaluation{
		InterceptorName: Name,
		BindingId:       CreateSandboxBindingID,
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("Evaluate code = %v, want InvalidArgument (err: %v)", status.Code(err), err)
	}
}

// The manifest binds only modify_operation; another phase is a contract
// violation and must not be silently allowed.
func TestEvaluateRejectsOtherPhases(t *testing.T) {
	srv, _ := newServer(policy.ModeEnforce)
	_, err := srv.Evaluate(context.Background(), &pb.InterceptorEvaluation{
		InterceptorName: Name,
		BindingId:       CreateSandboxBindingID,
		Phase: &pb.InterceptorEvaluation_Validate{
			Validate: &pb.ValidateEvaluation{},
		},
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("Evaluate code = %v, want InvalidArgument (err: %v)", status.Code(err), err)
	}
}

func TestSnapshotProviderProfilesUnimplemented(t *testing.T) {
	srv, _ := newServer(policy.ModeEnforce)
	_, err := srv.SnapshotProviderProfiles(context.Background(), &pb.ProviderProfileSnapshotRequest{})
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("SnapshotProviderProfiles code = %v, want Unimplemented (err: %v)", status.Code(err), err)
	}
}
