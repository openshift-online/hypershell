package reconciler

import (
	"testing"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
)

func TestGatewayNamespace(t *testing.T) {
	t.Run("returns the recorded namespace", func(t *testing.T) {
		gw := &pb.Gateway{
			Metadata:  &pb.ObjectReference{Id: "gw-123"},
			Name:      "my-gateway",
			Namespace: "openshell-0011223344556677",
		}
		ns, err := gatewayNamespace(gw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ns != "openshell-0011223344556677" {
			t.Errorf("namespace = %q, want %q", ns, "openshell-0011223344556677")
		}
	})

	t.Run("errors instead of guessing when namespace is empty", func(t *testing.T) {
		// A missing namespace must never be synthesized from the gateway name.
		// The real scheme is openshell-<hex(ksuid)> (set in Gateway.BeforeCreate),
		// so an openshell-<name> guess would target a different namespace; on the
		// delete path that could destroy the wrong (possibly live) namespace.
		// Callers are expected to refuse to act on the returned error.
		gw := &pb.Gateway{
			Metadata: &pb.ObjectReference{Id: "gw-456"},
			Name:     "my-gateway",
		}
		ns, err := gatewayNamespace(gw)
		if err == nil {
			t.Fatalf("expected error for empty namespace, got namespace %q", ns)
		}
		if ns != "" {
			t.Errorf("namespace = %q, want empty string on error", ns)
		}
	})
}
