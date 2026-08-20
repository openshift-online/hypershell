package reconciler

import (
	"testing"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestManagedDatabaseStatus(t *testing.T) {
	if got := managedDatabaseStatus(nil); got != "" {
		t.Fatalf("nil db: got %q, want empty", got)
	}

	ready := "Ready"
	if got := managedDatabaseStatus(&pb.ManagedDatabase{Status: &ready}); got != "Ready" {
		t.Fatalf("got %q, want Ready", got)
	}

	if got := managedDatabaseStatus(&pb.ManagedDatabase{}); got != "" {
		t.Fatalf("missing status: got %q, want empty", got)
	}
}

func TestCNPGClusterReadyFromObject(t *testing.T) {
	healthy := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"status": map[string]interface{}{
				"phase": "Cluster in healthy state",
			},
		},
	}
	if !cnpgClusterReadyFromObject(healthy) {
		t.Fatal("healthy phase should be ready")
	}

	readyInstances := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"status": map[string]interface{}{
				"instances":      int64(1),
				"readyInstances": int64(1),
			},
		},
	}
	if !cnpgClusterReadyFromObject(readyInstances) {
		t.Fatal("ready instances should be ready")
	}

	provisioning := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"status": map[string]interface{}{
				"phase":          "Setting up primary",
				"instances":      int64(1),
				"readyInstances": int64(0),
			},
		},
	}
	if cnpgClusterReadyFromObject(provisioning) {
		t.Fatal("provisioning cluster should not be ready")
	}

	if cnpgClusterReadyFromObject(nil) {
		t.Fatal("nil object should not be ready")
	}
}
