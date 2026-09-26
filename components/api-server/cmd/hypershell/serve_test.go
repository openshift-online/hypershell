package main

import (
	"testing"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api"
)

// The serve command must keep exposing the framework's environment flags,
// including the gRPC TLS ones the overlays set, or a hub would fail to start.
func TestServeCommandRegistersGRPCTLSFlags(t *testing.T) {
	cmd := newServeCommand(api.GetOpenAPISpec)
	if cmd.Use != "serve" {
		t.Fatalf("command use %q, want serve", cmd.Use)
	}
	for _, name := range []string{"grpc-enable-tls", "grpc-tls-cert-file", "grpc-tls-key-file", "grpc-server-bindaddress", "enable-grpc"} {
		if cmd.PersistentFlags().Lookup(name) == nil {
			t.Errorf("serve command does not register --%s", name)
		}
	}
}
