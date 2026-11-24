package gateways

import (
	"testing"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
)

// TestGatewayGRPCResourcesDeclareEveryMethod guards the gRPC authorization
// registration against drift: the framework interceptor denies any method with
// no declared resource (authz.UnaryInterceptor/StreamInterceptor return an
// "undeclared" PermissionDenied before HyperShell's authorizer runs). A new
// GatewayService RPC that is added to the proto but not registered in
// gatewayGRPCResources() would therefore be refused for every caller, including
// the control-plane identity it was written for (this is how
// FinalizeGatewayDeletion, GTW-DEL-10, first regressed). Asserting the
// registration covers every method and stream in the ServiceDesc keeps that
// failure a fast unit test instead of an integration-only surprise.
func TestGatewayGRPCResourcesDeclareEveryMethod(t *testing.T) {
	resources := gatewayGRPCResources()
	service := pb.GatewayService_ServiceDesc.ServiceName
	full := func(name string) string { return "/" + service + "/" + name }

	for _, m := range pb.GatewayService_ServiceDesc.Methods {
		if _, ok := resources[full(m.MethodName)]; !ok {
			t.Errorf("gRPC method %s has no declared authorization resource; add it to gatewayGRPCResources()", full(m.MethodName))
		}
	}
	for _, s := range pb.GatewayService_ServiceDesc.Streams {
		if _, ok := resources[full(s.StreamName)]; !ok {
			t.Errorf("gRPC stream %s has no declared authorization resource; add it to gatewayGRPCResources()", full(s.StreamName))
		}
	}
}
