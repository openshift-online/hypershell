package server

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

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

func TestDescribeManifest(t *testing.T) {
	srv := New("test-version")
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
	srv := New("test-version")

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

// Evaluate allows everything in the S2 skeleton; the resource policy lands
// with the S3 slice.
func TestEvaluateAllowsEverything(t *testing.T) {
	srv := New("test-version")
	proposed, err := structpb.NewStruct(map[string]any{
		"spec": map[string]any{
			"template": map[string]any{
				"resources": map[string]any{
					"requests": map[string]any{"cpu": "100m", "memory": "512Mi"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("building proposed operation: %v", err)
	}

	result, err := srv.Evaluate(context.Background(), &pb.InterceptorEvaluation{
		InterceptorName: Name,
		BindingId:       CreateSandboxBindingID,
		Service:         "openshell.v1.OpenShell",
		Method:          "CreateSandbox",
		Phase: &pb.InterceptorEvaluation_ModifyOperation{
			ModifyOperation: &pb.ModifyOperationEvaluation{ProposedOperation: proposed},
		},
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !result.GetAllowed() {
		t.Error("result.allowed = false, want true")
	}
	if len(result.GetPatches()) != 0 {
		t.Errorf("result.patches = %v, want none in the skeleton", result.GetPatches())
	}
	if result.GetReason() != "" {
		t.Errorf("result.reason = %q, want empty in the skeleton", result.GetReason())
	}
}

func TestEvaluateRejectsMissingPhase(t *testing.T) {
	srv := New("test-version")
	_, err := srv.Evaluate(context.Background(), &pb.InterceptorEvaluation{
		InterceptorName: Name,
		BindingId:       CreateSandboxBindingID,
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("Evaluate code = %v, want InvalidArgument (err: %v)", status.Code(err), err)
	}
}

func TestSnapshotProviderProfilesUnimplemented(t *testing.T) {
	srv := New("test-version")
	_, err := srv.SnapshotProviderProfiles(context.Background(), &pb.ProviderProfileSnapshotRequest{})
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("SnapshotProviderProfiles code = %v, want Unimplemented (err: %v)", status.Code(err), err)
	}
}
