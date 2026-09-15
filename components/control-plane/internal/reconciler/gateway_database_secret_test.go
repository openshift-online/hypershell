package reconciler

import (
	"context"
	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"testing"
)

func TestResolveControllerDatabaseSecret(t *testing.T) {
	// A nil gRPC connection makes any API lookup fail this test.
	r := &GatewayReconciler{controlPlaneNamespace: "hypershell", databaseSecret: "gateway-postgres"}
	for _, databaseID := range []string{"", "ignored-database-id"} {
		cfg, err := r.resolveDatabaseConfig(context.Background(), &pb.Gateway{DatabaseId: databaseID})
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Provider != "external" || cfg.ExternalDB.CredentialsNamespace != "hypershell" || cfg.ExternalDB.CredentialsSecretName != "gateway-postgres" {
			t.Fatalf("unexpected database configuration: %+v", cfg)
		}
		if cfg.ExternalDB.ManagedDatabaseID != "" || cfg.SourceNamespace != "" || cfg.CNPG.ClusterNamespace != "" {
			t.Fatalf("legacy database configuration is active: %+v", cfg)
		}
	}
}

func TestDatabaseSecretUnsetKeepsLegacyRequirement(t *testing.T) {
	r := &GatewayReconciler{}
	if _, err := r.resolveDatabaseConfig(context.Background(), &pb.Gateway{}); err == nil {
		t.Fatal("legacy database lookup must require database_id")
	}
}
