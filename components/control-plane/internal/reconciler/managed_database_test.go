package reconciler

import (
	"context"
	"errors"
	"strings"
	"testing"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
)

func TestNewManagedDatabaseReconcilerWithoutKubernetesClient(t *testing.T) {
	r := NewManagedDatabaseReconciler(nil, nil, nil, "")
	if r.hasCNPG {
		t.Fatal("hasCNPG = true without a Kubernetes client, want false")
	}
}

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

// A live event that arrives while a reconcile for the same ManagedDatabase is
// already in flight must be retained (not dropped) so the watch stream's
// no-replay guarantee never permanently loses it. A second live event while
// still busy must overwrite the first: only the latest pending state matters.
func TestManagedDatabaseReconciler_Handle_RetainsLatestPendingEventWhileBusy(t *testing.T) {
	r := &ManagedDatabaseReconciler{
		active:  make(map[string]struct{}),
		pending: make(map[string]watcher.Event[*pb.ManagedDatabase]),
	}
	const id = "db-1"

	// Simulate a reconcile already in flight for this resource.
	r.active[id] = struct{}{}

	first := watcher.Event[*pb.ManagedDatabase]{
		ResourceID: id,
		Type:       watcher.EventUpdated,
		Resource:   &pb.ManagedDatabase{Namespace: "first"},
	}
	if err := r.Handle(context.Background(), first); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	r.mu.Lock()
	pending, ok := r.pending[id]
	r.mu.Unlock()
	if !ok || pending.Resource.GetNamespace() != "first" {
		t.Fatalf("pending = %+v, ok=%v; want the first event retained", pending, ok)
	}

	second := watcher.Event[*pb.ManagedDatabase]{
		ResourceID: id,
		Type:       watcher.EventDeleted,
		Resource:   &pb.ManagedDatabase{Namespace: "second"},
	}
	if err := r.Handle(context.Background(), second); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	r.mu.Lock()
	pending, ok = r.pending[id]
	r.mu.Unlock()
	if !ok {
		t.Fatal("pending event was dropped, want the second event retained")
	}
	if pending.Type != watcher.EventDeleted || pending.Resource.GetNamespace() != "second" {
		t.Fatalf("pending = %+v, want the latest (second) event to survive, not the first", pending)
	}
}

func TestReconcileDeploymentDatabaseCredentialsConvergesWithoutRotatingPassword(t *testing.T) {
	const namespace = "database-ns"
	client := kubernetesfake.NewSimpleClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "openshell-db-credentials",
			Namespace: namespace,
			Labels:    map[string]string{"stale": "label"},
		},
		Data: map[string][]byte{
			"password": []byte("preserve-this-password"),
			"host":     []byte("stale-host"),
		},
	})
	r := &ManagedDatabaseReconciler{clientset: client}

	if err := r.reconcileDeploymentDatabaseCredentials(context.Background(), namespace, "openshell-db-credentials"); err != nil {
		t.Fatalf("reconcileDeploymentDatabaseCredentials: %v", err)
	}

	got, err := client.CoreV1().Secrets(namespace).Get(context.Background(), "openshell-db-credentials", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get reconciled Secret: %v", err)
	}
	if string(got.Data["password"]) != "preserve-this-password" {
		t.Fatalf("password was rotated: got %q", got.Data["password"])
	}
	if string(got.Data["host"]) != "openshell-gateway-db.database-ns.svc.cluster.local" {
		t.Fatalf("host = %q, want converged service DNS", got.Data["host"])
	}
	if !strings.Contains(string(got.Data["uri"]), "preserve-this-password") {
		t.Fatalf("uri was not reconciled from preserved password: %q", got.Data["uri"])
	}
	if got.Labels["hypershell.redhat.io/managed"] != "true" {
		t.Fatalf("managed label = %q, want true", got.Labels["hypershell.redhat.io/managed"])
	}
}

func TestDeploymentPostgresConfigForImage(t *testing.T) {
	tests := []struct {
		name     string
		image    string
		uid      int64
		userEnv  string
		dataPath string
		pgData   string
	}{
		{
			name:     "upstream postgres",
			image:    "postgres:18",
			uid:      999,
			userEnv:  "POSTGRES_USER",
			dataPath: "/var/lib/postgresql/data",
			pgData:   "/var/lib/postgresql/data/pgdata",
		},
		{
			name:     "Red Hat Hardened postgres uses upstream conventions",
			image:    "registry.access.redhat.com/hi/postgresql:18.4",
			uid:      26,
			userEnv:  "POSTGRES_USER",
			dataPath: "/var/lib/postgresql/data",
			pgData:   "/var/lib/postgresql/data/pgdata",
		},
		{
			name:     "legacy RHEL postgres",
			image:    "registry.redhat.io/rhel9/postgresql-15:latest",
			uid:      26,
			userEnv:  "POSTGRESQL_USER",
			dataPath: "/var/lib/pgsql/data",
			pgData:   "/var/lib/pgsql/data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deploymentPostgresConfigForImage(tt.image)
			if got.uid != tt.uid || got.userEnv != tt.userEnv || got.dataPath != tt.dataPath || got.pgData != tt.pgData {
				t.Fatalf("deploymentPostgresConfigForImage(%q) = %#v", tt.image, got)
			}
		})
	}
}

func TestReconcileDeploymentDatabaseUsesOpenShellImageAndPostgresSecurityContext(t *testing.T) {
	const namespace = "database-ns"
	t.Setenv("OPENSHELL_DATABASE_IMAGE", "registry.example/postgres:test")
	t.Setenv("HYPERSHELL_DATABASE_IMAGE", "registry.example/wrong:image")

	clientset := kubernetesfake.NewSimpleClientset()
	dynamicClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	r := &ManagedDatabaseReconciler{clientset: clientset, dynamicClient: dynamicClient, controlPlaneNamespace: "hypershell"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Avoid waiting for fake Deployment readiness after resources are applied.
	if err := r.reconcileDeploymentDatabase(ctx, &pb.ManagedDatabase{Namespace: namespace}); !errors.Is(err, context.Canceled) {
		t.Fatalf("reconcileDeploymentDatabase error = %v, want context cancellation after apply", err)
	}

	deploymentGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	deployment, err := dynamicClient.Resource(deploymentGVR).Namespace(namespace).Get(
		context.Background(), "openshell-gateway-db", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get reconciled Deployment: %v", err)
	}
	containers, found, err := unstructured.NestedSlice(deployment.Object, "spec", "template", "spec", "containers")
	if err != nil || !found || len(containers) != 1 {
		t.Fatalf("containers = %#v, found=%v, err=%v", containers, found, err)
	}
	container := containers[0].(map[string]interface{})
	if container["image"] != "registry.example/postgres:test" {
		t.Fatalf("image = %v, want OPENSHELL_DATABASE_IMAGE", container["image"])
	}
	podSecurity, found, err := unstructured.NestedMap(deployment.Object, "spec", "template", "spec", "securityContext")
	if err != nil || !found {
		t.Fatalf("pod securityContext not found: %v", err)
	}
	if podSecurity["runAsUser"] != int64(999) || podSecurity["fsGroup"] != int64(999) {
		t.Fatalf("pod securityContext = %#v, want PostgreSQL UID/fsGroup 999", podSecurity)
	}
	initContainers, found, err := unstructured.NestedSlice(deployment.Object, "spec", "template", "spec", "initContainers")
	if err != nil || !found || len(initContainers) != 1 {
		t.Fatalf("initContainers = %#v, found=%v, err=%v", initContainers, found, err)
	}
}

func TestApplyUnstructuredServicePreservesClusterIPAndConvergesSelector(t *testing.T) {
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "services"}
	existing := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata": map[string]interface{}{
			"name":      "openshell-gateway-db",
			"namespace": "database-ns",
		},
		"spec": map[string]interface{}{
			"type":      "ClusterIP",
			"clusterIP": "10.0.0.42",
			"selector":  map[string]interface{}{"app": "stale"},
			"ports": []interface{}{map[string]interface{}{
				"name": "postgresql", "port": int64(5432), "protocol": "TCP", "targetPort": "postgresql",
			}},
		},
	}}
	desired := existing.DeepCopy()
	unstructured.RemoveNestedField(desired.Object, "spec", "clusterIP")
	_ = unstructured.SetNestedMap(desired.Object, map[string]interface{}{"app": "openshell-gateway-db"}, "spec", "selector")
	desired.SetLabels(map[string]string{"hypershell.redhat.io/managed": "true"})

	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), existing)
	r := &ManagedDatabaseReconciler{dynamicClient: client}
	if err := r.applyUnstructured(context.Background(), desired); err != nil {
		t.Fatalf("applyUnstructured: %v", err)
	}

	got, err := client.Resource(gvr).Namespace("database-ns").Get(context.Background(), "openshell-gateway-db", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get reconciled Service: %v", err)
	}
	clusterIP, _, _ := unstructured.NestedString(got.Object, "spec", "clusterIP")
	if clusterIP != "10.0.0.42" {
		t.Fatalf("clusterIP = %q, want preserved immutable value", clusterIP)
	}
	selector, _, _ := unstructured.NestedStringMap(got.Object, "spec", "selector")
	if selector["app"] != "openshell-gateway-db" {
		t.Fatalf("selector = %v, want converged selector", selector)
	}
}

func TestApplyUnstructuredPVCPreservesBindingAndConvergesLabels(t *testing.T) {
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "persistentvolumeclaims"}
	existing := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "PersistentVolumeClaim",
		"metadata": map[string]interface{}{
			"name":      "openshell-gateway-db-data",
			"namespace": "database-ns",
		},
		"spec": map[string]interface{}{
			"accessModes": []interface{}{"ReadWriteOnce"},
			"volumeName":  "pvc-bound-volume",
			"resources": map[string]interface{}{
				"requests": map[string]interface{}{"storage": "1Gi"},
			},
		},
	}}
	desired := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "PersistentVolumeClaim",
		"metadata": map[string]interface{}{
			"name":      "openshell-gateway-db-data",
			"namespace": "database-ns",
			"labels":    map[string]interface{}{"hypershell.redhat.io/managed": "true"},
		},
		"spec": map[string]interface{}{
			"accessModes": []interface{}{"ReadWriteOnce"},
			"resources": map[string]interface{}{
				"requests": map[string]interface{}{"storage": "1Gi"},
			},
		},
	}}

	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), existing)
	r := &ManagedDatabaseReconciler{dynamicClient: client}
	if err := r.applyUnstructured(context.Background(), desired); err != nil {
		t.Fatalf("applyUnstructured: %v", err)
	}

	got, err := client.Resource(gvr).Namespace("database-ns").Get(context.Background(), "openshell-gateway-db-data", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get reconciled PVC: %v", err)
	}
	volumeName, _, _ := unstructured.NestedString(got.Object, "spec", "volumeName")
	if volumeName != "pvc-bound-volume" {
		t.Fatalf("volumeName = %q, want preserved binding", volumeName)
	}
	if got.GetLabels()["hypershell.redhat.io/managed"] != "true" {
		t.Fatalf("managed label = %q, want true", got.GetLabels()["hypershell.redhat.io/managed"])
	}
}

type fakeManagedDatabaseDeleteClient struct {
	err   error
	gotID string
}

func (f *fakeManagedDatabaseDeleteClient) DeleteManagedDatabase(_ context.Context, req *pb.DeleteManagedDatabaseRequest, _ ...grpc.CallOption) (*pb.DeleteManagedDatabaseResponse, error) {
	f.gotID = req.Id
	return &pb.DeleteManagedDatabaseResponse{}, f.err
}

func TestDeleteGatewayDeploymentDatabase(t *testing.T) {
	client := &fakeManagedDatabaseDeleteClient{}
	if err := deleteGatewayDeploymentDatabase(context.Background(), client, "db-1"); err != nil {
		t.Fatalf("deleteGatewayDeploymentDatabase: %v", err)
	}
	if client.gotID != "db-1" {
		t.Fatalf("deleted ID = %q, want db-1", client.gotID)
	}

	notFound := &fakeManagedDatabaseDeleteClient{err: status.Error(codes.NotFound, "gone")}
	if err := deleteGatewayDeploymentDatabase(context.Background(), notFound, "db-1"); err != nil {
		t.Fatalf("NotFound should be idempotent, got: %v", err)
	}

	unavailable := &fakeManagedDatabaseDeleteClient{err: status.Error(codes.Unavailable, "down")}
	err := deleteGatewayDeploymentDatabase(context.Background(), unavailable, "db-1")
	if err == nil || !errors.Is(err, unavailable.err) {
		t.Fatalf("error = %v, want wrapped unavailable error", err)
	}
}
