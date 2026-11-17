// Package server implements the openshell.gateway_interceptor.v1.
// GatewayInterceptor service for the sandbox-resource-interceptor sidecar.
//
// This is the S2 skeleton: Describe negotiates the extension contract and
// returns the manifest, and Evaluate allows every operation. The resource
// policy (fill missing CPU/memory requests at 250m/1Gi and deny below the
// floor) lands in the S3 slice.
package server

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/sandbox-resource-interceptor/pkg/api/grpc"
)

// Manifest values declared by this interceptor. The gateway refuses to start
// when the manifest audience does not match the operator-configured audience,
// and re-registers only on gateway restart.
const (
	// Name is the interceptor name registered in the gateway config.
	Name = "sandbox-resources"

	// ExpectedAudience is the exact JWT audience this service verifies on
	// inbound gateway calls.
	ExpectedAudience = "urn:openshell:extension:interceptor:sandbox-resources"

	// CreateSandboxRPC is the only bound RPC.
	CreateSandboxRPC = "openshell.v1.OpenShell/CreateSandbox"

	// CreateSandboxBindingID is the stable binding id for the
	// CreateSandbox modify_operation binding.
	CreateSandboxBindingID = "create-sandbox-modify-operation"

	// FailurePolicy is the default failure policy for all bindings. This is a
	// governance boundary: when the interceptor is unreachable or returns an
	// error, CreateSandbox is rejected rather than dispatched unpatched.
	FailurePolicy = "fail_closed"

	// ContractCapability is the gateway-interceptor family contract
	// capability every peer must advertise (openshell-core
	// extension_protocol.rs).
	ContractCapability = "openshell.gateway-interceptor.contract"

	// ProtocolMajor and ProtocolMinor are the gateway-interceptor protocol
	// version this implementation speaks.
	ProtocolMajor = 1
	ProtocolMinor = 0

	// ImplementationName identifies this service in negotiation logs.
	ImplementationName = "hypershell/sandbox-resource-interceptor"
)

// Server implements pb.GatewayInterceptorServer.
type Server struct {
	pb.UnimplementedGatewayInterceptorServer

	manifest *pb.InterceptorManifest
}

// New returns a Server reporting the given implementation version.
func New(implementationVersion string) *Server {
	return &Server{
		manifest: &pb.InterceptorManifest{
			Name:             Name,
			FailurePolicy:    FailurePolicy,
			ProviderProfiles: false,
			ExpectedAudience: ExpectedAudience,
			Bindings: []*pb.InterceptorBinding{
				{
					Id:            CreateSandboxBindingID,
					Selector:      &pb.InterceptorSelector{Rpc: CreateSandboxRPC},
					Phases:        []pb.GatewayInterceptorPhase{pb.GatewayInterceptorPhase_GATEWAY_INTERCEPTOR_PHASE_MODIFY_OPERATION},
					FailurePolicy: FailurePolicy,
				},
			},
			Extension: &pb.PeerMetadata{
				ProtocolVersion:       &pb.ProtocolVersion{Major: ProtocolMajor, Minor: ProtocolMinor},
				ImplementationName:    ImplementationName,
				ImplementationVersion: implementationVersion,
				SupportedCapabilities: []string{ContractCapability},
				RequiredCapabilities:  []string{ContractCapability},
			},
		},
	}
}

// Describe negotiates the gateway's protocol metadata and returns the
// interceptor manifest. Unmet gateway requirements are rejected, mirroring the
// upstream negotiate() contract.
func (s *Server) Describe(_ context.Context, req *pb.DescribeRequest) (*pb.InterceptorManifest, error) {
	if err := negotiateGatewayMetadata(req.GetGateway()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	return s.manifest, nil
}

// Evaluate decides one operation phase. The S2 skeleton allows everything;
// the resource policy lands with the S3 slice.
func (s *Server) Evaluate(_ context.Context, evaluation *pb.InterceptorEvaluation) (*pb.InterceptorResult, error) {
	if evaluation.GetPhase() == nil {
		return nil, status.Error(codes.InvalidArgument, "evaluation carries no phase payload")
	}
	return &pb.InterceptorResult{Allowed: true}, nil
}

// SnapshotProviderProfiles is not implemented: the manifest advertises
// provider_profiles = false.
func (s *Server) SnapshotProviderProfiles(context.Context, *pb.ProviderProfileSnapshotRequest) (*pb.ProviderProfileSnapshot, error) {
	return nil, status.Error(codes.Unimplemented, "sandbox-resources does not vend provider profiles")
}

// negotiateGatewayMetadata validates the gateway's PeerMetadata: the
// interceptor protocol must be 1.x and both peers must advertise the
// gateway-interceptor contract capability. Anything else is an unmet
// requirement the caller must reject.
func negotiateGatewayMetadata(gateway *pb.PeerMetadata) error {
	if gateway == nil {
		return fmt.Errorf("gateway did not provide protocol metadata; expected protocol %d.%d and capability %s",
			ProtocolMajor, ProtocolMinor, ContractCapability)
	}
	version := gateway.GetProtocolVersion()
	if version == nil {
		return fmt.Errorf("gateway did not provide a protocol version; expected protocol %d.%d and capability %s",
			ProtocolMajor, ProtocolMinor, ContractCapability)
	}
	if version.GetMajor() != ProtocolMajor {
		return fmt.Errorf("gateway uses unsupported protocol %d.%d; this interceptor supports %d.%d",
			version.GetMajor(), version.GetMinor(), ProtocolMajor, ProtocolMinor)
	}
	if !hasCapability(gateway.GetSupportedCapabilities(), ContractCapability) {
		return fmt.Errorf("gateway is missing required capability %s", ContractCapability)
	}
	for _, required := range gateway.GetRequiredCapabilities() {
		if required != ContractCapability {
			return fmt.Errorf("gateway requires capability %s this interceptor does not support", required)
		}
	}
	return nil
}

func hasCapability(capabilities []string, want string) bool {
	for _, capability := range capabilities {
		if capability == want {
			return true
		}
	}
	return false
}
