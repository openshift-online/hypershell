package gateways

import (
	"reflect"
	"strings"
	"testing"

	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// TestGatewayFieldCensus enforces the default-deny guarantee (PLAT-DATA-23.07): every
// field on the Gateway model must be explicitly classified as user-writable or
// control-plane-only. A new field added without a decision FAILS this test, so the
// ownership rule is enforced in CI rather than by review alone.
func TestGatewayFieldCensus(t *testing.T) {
	gwType := reflect.TypeOf(Gateway{})
	for i := 0; i < gwType.NumField(); i++ {
		field := gwType.Field(i)
		// Embedded framework metadata (api.Meta, TraceMeta) is server-owned wholesale:
		// ids, timestamps, and trace context are never caller-authored.
		if field.Anonymous {
			continue
		}
		tag := field.Tag.Get("json")
		name := strings.Split(tag, ",")[0]
		if name == "" || name == "-" {
			t.Fatalf("Gateway field %q has no json tag; classify it in gatewayFieldOwnership", field.Name)
		}
		if _, ok := gatewayFieldOwnership[name]; !ok {
			t.Fatalf("Gateway field %q (json %q) is not classified in gatewayFieldOwnership; "+
				"a new gateway field is control-plane-only by default and must be declared "+
				"there (PLAT-DATA-23.07)", field.Name, name)
		}
	}
}

func TestEnforceGatewayFieldPolicyCreate(t *testing.T) {
	img := "quay.io/example/image:v1"
	oidc := `{"issuer":"https://kc"}`

	tests := []struct {
		name      string
		kind      rbac.GatewayWriterKind
		gateway   *Gateway
		wantField string // "" means accepted
	}{
		{"user with only name+cluster accepted", rbac.GatewayWriterUser, &Gateway{Name: "gw", ClusterId: "c1"}, ""},
		{"user with image rejected", rbac.GatewayWriterUser, &Gateway{Name: "gw", Image: &img}, "image"},
		{"import with oidc rejected", rbac.GatewayWriterImport, &Gateway{Name: "gw", Oidc: &oidc}, "oidc"},
		{"control-plane with image accepted", rbac.GatewayWriterControlPlane, &Gateway{Name: "gw", Image: &img}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := enforceGatewayFieldPolicyCreate(tc.kind, tc.gateway)
			assertFieldPolicy(t, err, tc.wantField)
		})
	}
}

func TestEnforceGatewayFieldPolicyUpdate(t *testing.T) {
	imgA, imgB := "img:a", "img:b"
	stored := &Gateway{Name: "gw", ClusterId: "c1", Image: &imgA}

	tests := []struct {
		name      string
		kind      rbac.GatewayWriterKind
		incoming  *Gateway
		wantField string
	}{
		{"user rename accepted", rbac.GatewayWriterUser, &Gateway{Name: "renamed", ClusterId: "c1", Image: &imgA}, ""},
		{"user resend unchanged image is no-op", rbac.GatewayWriterUser, &Gateway{Name: "renamed", ClusterId: "c1", Image: &imgA}, ""},
		{"user changing image rejected", rbac.GatewayWriterUser, &Gateway{Name: "gw", ClusterId: "c1", Image: &imgB}, "image"},
		{"user changing cluster_id rejected", rbac.GatewayWriterUser, &Gateway{Name: "gw", ClusterId: "c2", Image: &imgA}, "cluster_id"},
		{"user setting status rejected", rbac.GatewayWriterUser, &Gateway{Name: "gw", ClusterId: "c1", Image: &imgA, Status: strPtr("Healthy")}, "status"},
		{"import changing image rejected", rbac.GatewayWriterImport, &Gateway{Name: "gw", ClusterId: "c1", Image: &imgB}, "image"},
		{"control-plane changing image accepted", rbac.GatewayWriterControlPlane, &Gateway{Name: "gw", ClusterId: "c1", Image: &imgB}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := enforceGatewayFieldPolicyUpdate(tc.kind, stored, tc.incoming)
			assertFieldPolicy(t, err, tc.wantField)
		})
	}
}

func assertFieldPolicy(t *testing.T, err *errors.ServiceError, wantField string) {
	t.Helper()
	if wantField == "" {
		if err != nil {
			t.Fatalf("expected the write to be accepted, got %v", err)
		}
		return
	}
	if err == nil {
		t.Fatalf("expected rejection naming %q, got nil", wantField)
	}
	if !strings.Contains(err.Error(), wantField) {
		t.Fatalf("expected the rejection to name %q, got %q", wantField, err.Error())
	}
}
