package gateways

import (
	"testing"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
)

func strptr(s string) *string { return &s }

// TestConvertGatewayCreateAllowlist locks the create mapping to exactly the
// user-writable allowlist (PLAT-DATA-23): name, the create-only
// dedicated_route request flag, and additional_ca_bundle (user-authored trust
// material). A caller-supplied platform-managed field must never be copied
// into the persisted model.
func TestConvertGatewayCreateAllowlist(t *testing.T) {
	request := openapi.GatewayCreateRequest{
		Name:               "my-gateway",
		DedicatedRoute:     openapi.PtrBool(true),
		AdditionalCaBundle: strptr("-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"),
	}
	gateway := ConvertGateway(request)
	if gateway.Name != "my-gateway" {
		t.Fatalf("name = %q, want my-gateway", gateway.Name)
	}
	if gateway.DedicatedRoute == nil || !*gateway.DedicatedRoute {
		t.Fatalf("dedicated_route = %v, want true", gateway.DedicatedRoute)
	}
	if gateway.AdditionalCABundle == nil || *gateway.AdditionalCABundle != *request.AdditionalCaBundle {
		t.Fatalf("additional_ca_bundle = %v, want the request bundle", gateway.AdditionalCABundle)
	}
}

func TestPresentGatewayAdditionalCABundle(t *testing.T) {
	bundle := "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"
	gateway := &Gateway{Name: "my-gateway", AdditionalCABundle: &bundle}
	presented := PresentGateway(gateway, "", true, true)
	if presented.AdditionalCaBundle == nil || *presented.AdditionalCaBundle != bundle {
		t.Fatalf("presented additional_ca_bundle = %v, want the stored bundle", presented.AdditionalCaBundle)
	}

	// An absent bundle presents as nil, not as an empty string, so clients can
	// distinguish "no user trust declared" from "trust cleared to empty".
	gateway = &Gateway{Name: "my-gateway"}
	presented = PresentGateway(gateway, "", true, true)
	if presented.AdditionalCaBundle != nil {
		t.Fatalf("presented additional_ca_bundle = %v, want nil", presented.AdditionalCaBundle)
	}
}
