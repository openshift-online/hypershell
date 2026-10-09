package e2e

import (
	"testing"
	"time"

	sdktypes "github.com/openshift-online/hypershell/components/sdk-go/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestValidateProvisioningConditions(t *testing.T) {
	complete := []map[string]any{
		{"type": "EnvironmentReady", "condition_status": "Complete"},
		{"type": "DatabaseReady", "condition_status": "Complete"},
		{"type": "GatewayDeployed", "condition_status": "Complete"},
		{"type": "GatewayHealthy", "condition_status": "Complete"},
	}
	if err := validateProvisioningConditions(complete); err != nil {
		t.Fatalf("complete conditions rejected: %v", err)
	}

	for name, conditions := range map[string][]map[string]any{
		"empty":      {},
		"missing":    complete[:3],
		"incomplete": append(append([]map[string]any{}, complete[:3]...), map[string]any{"type": "GatewayHealthy", "condition_status": "Pending"}),
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateProvisioningConditions(conditions); err == nil {
				t.Fatal("invalid conditions accepted")
			}
		})
	}
}

func TestValidateGatewayDatabaseSecret(t *testing.T) {
	valid := map[string][]byte{
		"sslmode": []byte("require"),
		"uri":     []byte("postgresql://gateway@example/db?sslmode=require"),
	}
	if err := validateGatewayDatabaseSecret(valid); err != nil {
		t.Fatalf("valid database secret rejected: %v", err)
	}
	valid["sslrootcert"] = []byte("unexpected")
	if err := validateGatewayDatabaseSecret(valid); err == nil {
		t.Fatal("database secret with sslrootcert accepted")
	}
}

func TestCertificateReady(t *testing.T) {
	certificate := &unstructured.Unstructured{Object: map[string]any{
		"status": map[string]any{"conditions": []any{
			map[string]any{"type": "Ready", "status": "True"},
		}},
	}}
	if !certificateReady(certificate) {
		t.Fatal("Ready=True certificate reported not ready")
	}
}

func TestManagedKubeContextEnvKey(t *testing.T) {
	if got, want := managedKubeContextEnvKey("west-1.example/edge"), "E2E_MANAGED_KUBECONTEXT_WEST_1_EXAMPLE_EDGE"; got != want {
		t.Fatalf("managedKubeContextEnvKey() = %q, want %q", got, want)
	}
}

func TestValidateGatewayServiceAccountClaims(t *testing.T) {
	account := &sdktypes.OpenShellGatewayServiceAccountCreateResponse{
		Subject: "service-subject",
		Credential: sdktypes.OpenShellGatewayServiceAccountCredential{
			Issuer: "https://issuer.example", Audience: "gateway-client",
		},
	}
	claims := map[string]any{
		"iss":        "https://issuer.example",
		"sub":        "service-subject",
		"aud":        []any{"gateway-client"},
		"exp":        float64(time.Now().Unix() + 120),
		"hypershell": map[string]any{"roles": []any{"openshell-user", "openshell-admin"}},
	}
	if err := validateGatewayServiceAccountClaims(claims, account); err != nil {
		t.Fatalf("valid gateway service-account claims rejected: %v", err)
	}
	claims["aud"] = []any{"different-client"}
	if err := validateGatewayServiceAccountClaims(claims, account); err == nil {
		t.Fatal("wrong gateway audience accepted")
	}
}
