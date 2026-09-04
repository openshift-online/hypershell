package reconciler

import (
	"bytes"
	"context"
	cryptoRand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"
	"unicode"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/exposure"
	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
	"github.com/openshift-online/hypershell/components/control-plane/internal/keycloak"
	cpotel "github.com/openshift-online/hypershell/components/control-plane/internal/otel"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

type ManagedClusterReconciler struct {
	mu     sync.Mutex
	active map[string]struct{}
}

func NewManagedClusterReconciler() *ManagedClusterReconciler {
	return &ManagedClusterReconciler{active: make(map[string]struct{})}
}

func (r *ManagedClusterReconciler) Handle(ctx context.Context, event watcher.Event[*pb.ManagedCluster]) error {
	r.mu.Lock()
	if _, ok := r.active[event.ResourceID]; ok {
		r.mu.Unlock()
		return nil
	}
	r.active[event.ResourceID] = struct{}{}
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.active, event.ResourceID)
		r.mu.Unlock()
	}()

	_, endSpan := cpotel.StartReconcileSpan(ctx, "ManagedCluster", event.Type.String())
	defer func() { endSpan(nil) }()

	log.Printf("INFO reconciling ManagedCluster %s (event=%d)", event.ResourceID, event.Type)
	return nil
}

type ManagedDatabaseReconciler struct {
	mu                    sync.Mutex
	active                map[string]struct{}
	pending               map[string]watcher.Event[*pb.ManagedDatabase]
	dynamicClient         dynamic.Interface
	clientset             kubernetes.Interface
	grpcConn              *grpc.ClientConn
	hasCNPG               bool
	isOpenShift           bool
	controlPlaneNamespace string
	lastSeen              map[string]*pb.ManagedDatabase
}

func NewManagedDatabaseReconciler(
	dynamicClient dynamic.Interface,
	clientset kubernetes.Interface,
	grpcConn *grpc.ClientConn,
	controlPlaneNamespace string,
) *ManagedDatabaseReconciler {
	hasCNPG := false
	isOpenShift := false
	if clientset != nil {
		hasCNPG = gateway.DetectCNPG(clientset)
		// DetectOpenShift accepts the concrete client used by the controller.
		// Retain the reconciler interface-typed client for fake-client tests.
		if concreteClientset, ok := clientset.(*kubernetes.Clientset); ok {
			isOpenShift = gateway.DetectOpenShift(concreteClientset)
		}
	}
	return &ManagedDatabaseReconciler{
		active:                make(map[string]struct{}),
		pending:               make(map[string]watcher.Event[*pb.ManagedDatabase]),
		lastSeen:              make(map[string]*pb.ManagedDatabase),
		dynamicClient:         dynamicClient,
		clientset:             clientset,
		grpcConn:              grpcConn,
		hasCNPG:               hasCNPG,
		isOpenShift:           isOpenShift,
		controlPlaneNamespace: controlPlaneNamespace,
	}
}

// Handle reconciles one ManagedDatabase event, serializing per resource ID.
func (r *ManagedDatabaseReconciler) Handle(ctx context.Context, event watcher.Event[*pb.ManagedDatabase]) error {
	if event.Type != watcher.EventDeleted && event.Resource != nil {
		r.rememberManagedDatabase(event.ResourceID, event.Resource)
	}
	r.mu.Lock()
	if _, busy := r.active[event.ResourceID]; busy {
		r.pending[event.ResourceID] = event
		r.mu.Unlock()
		return nil
	}
	r.active[event.ResourceID] = struct{}{}
	r.mu.Unlock()
	firstErr := r.handleOne(ctx, event)
	current := event
	for {
		r.mu.Lock()
		next, hasPending := r.pending[current.ResourceID]
		if hasPending {
			delete(r.pending, current.ResourceID)
		} else {
			delete(r.active, current.ResourceID)
		}
		r.mu.Unlock()
		if !hasPending {
			break
		}
		current = next
		if err := r.handleOne(ctx, current); err != nil {
			log.Printf("ERROR handling pending managed database %s: %v", current.ResourceID, err)
		}
	}
	return firstErr
}

func (r *ManagedDatabaseReconciler) handleOne(ctx context.Context, event watcher.Event[*pb.ManagedDatabase]) (reconcileErr error) {
	ctx, endSpan := cpotel.StartReconcileSpan(ctx, "ManagedDatabase", event.Type.String())
	defer func() { endSpan(reconcileErr) }()

	if r.clientset == nil || r.dynamicClient == nil {
		return fmt.Errorf("reconcile ManagedDatabase %s: Kubernetes typed and dynamic clients are required", event.ResourceID)
	}
	db := event.Resource
	if event.Type == watcher.EventDeleted {
		if db == nil {
			db = r.lastSeenManagedDatabase(event.ResourceID)
			if db == nil {
				return fmt.Errorf("delete ManagedDatabase %s: event has no resource and no last-seen resource is available", event.ResourceID)
			}
		}
		// Retain the authoritative tombstone until cleanup succeeds so a retry can
		// still proceed if a mixed-version API server later sends only the ID.
		r.rememberManagedDatabase(event.ResourceID, db)
	} else {
		if db == nil {
			log.Printf("WARN ManagedDatabase event %s has nil resource, skipping", event.ResourceID)
			return nil
		}
		r.rememberManagedDatabase(event.ResourceID, db)
		if r.grpcConn == nil {
			return fmt.Errorf("reconcile ManagedDatabase %s: gRPC client is required before updating status", event.ResourceID)
		}
	}
	var err error
	switch db.Provider {
	case "cnpg":
		err = r.handleCNPGDatabase(ctx, event, db)
	case "deployment":
		err = r.handleDeploymentDatabase(ctx, event, db)
	default:
		log.Printf("WARN ManagedDatabase %s has unsupported provider %q, skipping", event.ResourceID, db.Provider)
		return nil
	}
	if err == nil && event.Type == watcher.EventDeleted {
		r.forgetManagedDatabase(event.ResourceID)
	}
	return err
}

func (r *ManagedDatabaseReconciler) rememberManagedDatabase(id string, db *pb.ManagedDatabase) {
	if id == "" || db == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.lastSeen == nil {
		r.lastSeen = make(map[string]*pb.ManagedDatabase)
	}
	r.lastSeen[id] = proto.Clone(db).(*pb.ManagedDatabase)
}

func (r *ManagedDatabaseReconciler) lastSeenManagedDatabase(id string) *pb.ManagedDatabase {
	r.mu.Lock()
	defer r.mu.Unlock()
	if db := r.lastSeen[id]; db != nil {
		return proto.Clone(db).(*pb.ManagedDatabase)
	}
	return nil
}

func (r *ManagedDatabaseReconciler) forgetManagedDatabase(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.lastSeen, id)
}
func (r *ManagedDatabaseReconciler) handleCNPGDatabase(ctx context.Context, event watcher.Event[*pb.ManagedDatabase], db *pb.ManagedDatabase) error {
	if event.Type == watcher.EventDeleted {
		log.Printf("INFO ManagedDatabase %s deleted, cleaning up CNPG cluster in namespace %s", event.ResourceID, db.Namespace)
		if err := r.deleteCNPGCluster(ctx, db.Namespace); err != nil {
			return fmt.Errorf("delete CNPG database for ManagedDatabase %s: %w", db.Name, err)
		}
		return nil
	}

	log.Printf("INFO reconciling ManagedDatabase %s name=%s namespace=%s provider=cnpg (event=%d)",
		event.ResourceID, db.Name, db.Namespace, event.Type)

	if !r.hasCNPG {
		r.updateManagedDatabaseStatusIfChanged(ctx, event.ResourceID, managedDatabaseStatus(db), "Failed: CNPG operator not available")
		return fmt.Errorf("CNPG operator is required but not available on the cluster")
	}

	clusterName := managedDatabaseCNPGClusterName()
	currentStatus := managedDatabaseStatus(db)
	clusterReady, err := r.isCNPGClusterReady(ctx, db.Namespace, clusterName)
	if err != nil {
		return fmt.Errorf("check CNPG Cluster readiness for ManagedDatabase %s: %w", db.Name, err)
	}

	if clusterReady {
		if currentStatus == "Ready" {
			log.Printf("DEBUG ManagedDatabase %s status=Ready and CNPG cluster healthy, skipping reconciliation", event.ResourceID)
			return nil
		}
		r.updateManagedDatabaseStatusIfChanged(ctx, event.ResourceID, currentStatus, "Ready")
		log.Printf("INFO ManagedDatabase %s CNPG cluster already ready in namespace %s", event.ResourceID, db.Namespace)
		return nil
	}

	if currentStatus == "Ready" {
		log.Printf("WARN ManagedDatabase %s status=Ready but CNPG cluster not healthy, re-reconciling", event.ResourceID)
	}

	r.updateManagedDatabaseStatusIfChanged(ctx, event.ResourceID, currentStatus, "Provisioning")

	if err := r.reconcileCNPGCluster(ctx, db); err != nil {
		r.updateManagedDatabaseStatusIfChanged(ctx, event.ResourceID, "Provisioning", fmt.Sprintf("Failed: %v", err))
		return fmt.Errorf("reconcile CNPG cluster for ManagedDatabase %s: %w", db.Name, err)
	}

	r.updateManagedDatabaseStatusIfChanged(ctx, event.ResourceID, "Provisioning", "Ready")
	log.Printf("INFO ManagedDatabase %s CNPG cluster provisioned in namespace %s", event.ResourceID, db.Namespace)
	return nil
}

func (r *ManagedDatabaseReconciler) handleDeploymentDatabase(ctx context.Context, event watcher.Event[*pb.ManagedDatabase], db *pb.ManagedDatabase) error {
	if event.Type == watcher.EventDeleted {
		log.Printf("INFO ManagedDatabase %s deleted, cleaning up deployment database in namespace %s", event.ResourceID, db.Namespace)
		if err := r.deleteDeploymentDatabase(ctx, db.Namespace); err != nil {
			return fmt.Errorf("delete deployment database for ManagedDatabase %s: %w", db.Name, err)
		}
		return nil
	}

	log.Printf("INFO reconciling ManagedDatabase %s name=%s namespace=%s provider=deployment (event=%d)",
		event.ResourceID, db.Name, db.Namespace, event.Type)

	currentStatus := managedDatabaseStatus(db)

	// Readiness is observed after desired state has converged. A Ready status is
	// not a reason to skip reconciliation: image, security, labels, resources, and
	// copied connection data may have changed since the previous event. However,
	// repeatedly writing Ready -> Provisioning -> Ready creates a self-sustaining
	// watch-event storm because each status write emits another ManagedDatabase
	// event. Keep Ready while checking an already-ready resource; transition to
	// Provisioning only when work is not already reported ready.
	reconcileStatus := currentStatus
	if currentStatus != "Ready" {
		r.updateManagedDatabaseStatusIfChanged(ctx, event.ResourceID, currentStatus, "Provisioning")
		reconcileStatus = "Provisioning"
	}

	if err := r.reconcileDeploymentDatabase(ctx, db); err != nil {
		r.updateManagedDatabaseStatusIfChanged(ctx, event.ResourceID, reconcileStatus, fmt.Sprintf("Failed: %v", err))
		return fmt.Errorf("reconcile deployment database for ManagedDatabase %s: %w", db.Name, err)
	}

	r.updateManagedDatabaseStatusIfChanged(ctx, event.ResourceID, reconcileStatus, "Ready")
	log.Printf("INFO ManagedDatabase %s deployment database provisioned in namespace %s", event.ResourceID, db.Namespace)
	return nil
}

func (r *ManagedDatabaseReconciler) reconcileCNPGCluster(ctx context.Context, db *pb.ManagedDatabase) error {
	namespace := db.Namespace

	if err := gateway.EnsureManagedNamespace(ctx, r.clientset, namespace, r.controlPlaneNamespace); err != nil {
		return fmt.Errorf("ensure namespace %s: %w", namespace, err)
	}

	clusterName := managedDatabaseCNPGClusterName()

	spec := map[string]interface{}{
		"instances": int64(1),
		"storage": map[string]interface{}{
			"size": "1Gi",
		},
		"resources": map[string]interface{}{
			"requests": map[string]interface{}{
				"memory": "256Mi",
			},
			"limits": map[string]interface{}{
				"memory": "512Mi",
			},
		},
	}

	if image := os.Getenv("OPENSHELL_DATABASE_IMAGE"); image != "" {
		spec["imageName"] = image
	}

	cluster := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "postgresql.cnpg.io/v1",
			"kind":       "Cluster",
			"metadata": map[string]interface{}{
				"name":      clusterName,
				"namespace": namespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/managed-by": "hypershell-control-plane",
					"hypershell.redhat.io/managed": "true",
				},
			},
			"spec": spec,
		},
	}

	clusterGVR := cnpgClusterGVR()
	existing, err := r.dynamicClient.Resource(clusterGVR).Namespace(namespace).Get(ctx, clusterName, metav1.GetOptions{})
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return fmt.Errorf("get CNPG Cluster: %w", err)
		}
		if _, err := r.dynamicClient.Resource(clusterGVR).Namespace(namespace).Create(ctx, cluster, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create CNPG Cluster: %w", err)
		}
		log.Printf("INFO created CNPG Cluster %s in namespace %s", clusterName, namespace)
	} else if cnpgClusterReadyFromObject(existing) {
		log.Printf("INFO CNPG Cluster %s in namespace %s already exists and is ready", clusterName, namespace)
	} else {
		cluster.SetResourceVersion(existing.GetResourceVersion())
		if _, err := r.dynamicClient.Resource(clusterGVR).Namespace(namespace).Update(ctx, cluster, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update CNPG Cluster: %w", err)
		}
		log.Printf("INFO updated CNPG Cluster %s in namespace %s", clusterName, namespace)
	}

	if err := r.waitForCNPGClusterReady(ctx, namespace, clusterName, 3*time.Minute); err != nil {
		return fmt.Errorf("wait for CNPG Cluster ready: %w", err)
	}

	return nil
}

func (r *ManagedDatabaseReconciler) waitForCNPGClusterReady(ctx context.Context, namespace, name string, timeout time.Duration) error {
	if ready, err := r.isCNPGClusterReady(ctx, namespace, name); err != nil {
		return err
	} else if ready {
		return nil
	}

	deadline := time.After(timeout)
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timed out waiting for CNPG Cluster %s/%s to become ready", namespace, name)
		case <-ticker.C:
			ready, err := r.isCNPGClusterReady(ctx, namespace, name)
			if err != nil {
				if k8serrors.IsNotFound(err) {
					log.Printf("DEBUG CNPG Cluster %s/%s not found yet", namespace, name)
					continue
				}
				return err
			}
			if ready {
				return nil
			}
		}
	}
}

func (r *ManagedDatabaseReconciler) isCNPGClusterReady(ctx context.Context, namespace, name string) (bool, error) {
	obj, err := r.dynamicClient.Resource(cnpgClusterGVR()).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return cnpgClusterReadyFromObject(obj), nil
}

func managedDatabaseCNPGClusterName() string {
	return "openshell-db"
}

func managedDatabaseStatus(db *pb.ManagedDatabase) string {
	if db == nil || db.Status == nil {
		return ""
	}
	return *db.Status
}

func cnpgClusterReadyFromObject(obj *unstructured.Unstructured) bool {
	if obj == nil {
		return false
	}
	phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
	if phase == "Cluster in healthy state" || phase == "Cluster is Ready" {
		log.Printf("INFO CNPG Cluster %s/%s is ready (phase=%s)", obj.GetNamespace(), obj.GetName(), phase)
		return true
	}
	readyInstances, _, _ := unstructured.NestedInt64(obj.Object, "status", "readyInstances")
	instances, _, _ := unstructured.NestedInt64(obj.Object, "status", "instances")
	if readyInstances > 0 && readyInstances >= instances {
		log.Printf("INFO CNPG Cluster %s/%s is ready (readyInstances=%d/%d)", obj.GetNamespace(), obj.GetName(), readyInstances, instances)
		return true
	}
	log.Printf("DEBUG CNPG Cluster %s/%s not ready yet (phase=%s ready=%d/%d)", obj.GetNamespace(), obj.GetName(), phase, readyInstances, instances)
	return false
}

func (r *ManagedDatabaseReconciler) deleteCNPGCluster(ctx context.Context, namespace string) error {
	clusterName := managedDatabaseCNPGClusterName()
	var errs []error
	if err := r.dynamicClient.Resource(cnpgClusterGVR()).Namespace(namespace).Delete(ctx, clusterName, metav1.DeleteOptions{}); err != nil {
		if !k8serrors.IsNotFound(err) {
			errs = append(errs, fmt.Errorf("delete CNPG Cluster %s/%s: %w", namespace, clusterName, err))
		}
	} else {
		log.Printf("INFO deleted CNPG Cluster %s/%s", namespace, clusterName)
	}
	if err := r.clientset.CoreV1().Namespaces().Delete(ctx, namespace, metav1.DeleteOptions{}); err != nil {
		if !k8serrors.IsNotFound(err) {
			errs = append(errs, fmt.Errorf("delete namespace %s: %w", namespace, err))
		}
	} else {
		log.Printf("INFO deleted namespace %s", namespace)
	}
	return errors.Join(errs...)
}
func (r *ManagedDatabaseReconciler) reconcileDeploymentDatabaseNamespace(ctx context.Context, namespace string) error {
	return gateway.EnsureManagedNamespace(ctx, r.clientset, namespace, r.controlPlaneNamespace)
}

func (r *ManagedDatabaseReconciler) reconcileDeploymentDatabaseCredentials(ctx context.Context, namespace, name string) error {
	secrets := r.clientset.CoreV1().Secrets(namespace)
	existing, err := secrets.Get(ctx, name, metav1.GetOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("get database credentials secret %s/%s: %w", namespace, name, err)
	}

	password := ""
	if err == nil {
		password = string(existing.Data["password"])
	}
	if password == "" {
		passwordBytes := make([]byte, 32)
		if _, err := cryptoRand.Read(passwordBytes); err != nil {
			return fmt.Errorf("generate database password: %w", err)
		}
		password = hex.EncodeToString(passwordBytes)
	}

	host := fmt.Sprintf("openshell-gateway-db.%s.svc.cluster.local", namespace)
	port := "5432"
	dbName := "openshell"
	dbUser := "openshell"
	dbURI := fmt.Sprintf("postgresql://%s:%s@%s:%s/%s?sslmode=disable",
		dbUser, url.QueryEscape(password), host, port, dbName)
	desiredData := map[string][]byte{
		"host":     []byte(host),
		"port":     []byte(port),
		"dbname":   []byte(dbName),
		"user":     []byte(dbUser),
		"password": []byte(password),
		"uri":      []byte(dbURI),
	}
	desiredLabels := map[string]string{
		"app.kubernetes.io/name":       "openshell",
		"app.kubernetes.io/component":  "database",
		"app.kubernetes.io/managed-by": "hypershell-control-plane",
		"hypershell.redhat.io/managed": "true",
	}

	if k8serrors.IsNotFound(err) {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: desiredLabels},
			Type:       corev1.SecretTypeOpaque,
			Data:       desiredData,
		}
		if _, err := secrets.Create(ctx, secret, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create database credentials secret %s/%s: %w", namespace, name, err)
		}
		log.Printf("INFO created database credentials secret %s in %s", name, namespace)
		return nil
	}

	updated := existing.DeepCopy()
	if updated.Labels == nil {
		updated.Labels = map[string]string{}
	}
	for key, value := range desiredLabels {
		updated.Labels[key] = value
	}
	updated.Type = corev1.SecretTypeOpaque
	updated.Data = desiredData
	if reflect.DeepEqual(existing.Labels, updated.Labels) && existing.Type == updated.Type && reflect.DeepEqual(existing.Data, updated.Data) {
		return nil
	}
	if _, err := secrets.Update(ctx, updated, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update database credentials secret %s/%s: %w", namespace, name, err)
	}
	log.Printf("INFO updated database credentials secret %s in %s", name, namespace)
	return nil
}

type deploymentPostgresImageConfig struct {
	uid      int64
	userEnv  string
	passEnv  string
	dbEnv    string
	dataPath string
	pgData   string
}

func deploymentPostgresConfigForImage(image string) deploymentPostgresImageConfig {
	lowerImage := strings.ToLower(image)
	legacyRHEL := strings.Contains(lowerImage, "rhel") && strings.Contains(lowerImage, "postgresql-")
	redHatHardened := strings.Contains(lowerImage, "registry.access.redhat.com/hi/postgresql")

	if legacyRHEL {
		return deploymentPostgresImageConfig{
			uid:      26,
			userEnv:  "POSTGRESQL_USER",
			passEnv:  "POSTGRESQL_PASSWORD",
			dbEnv:    "POSTGRESQL_DATABASE",
			dataPath: "/var/lib/pgsql/data",
			pgData:   "/var/lib/pgsql/data",
		}
	}

	uid := int64(999)
	if redHatHardened {
		// Red Hat Hardened PostgreSQL uses UID/GID 26 but follows the upstream
		// POSTGRES_* environment and /var/lib/postgresql/data conventions.
		uid = 26
	}
	return deploymentPostgresImageConfig{
		uid:      uid,
		userEnv:  "POSTGRES_USER",
		passEnv:  "POSTGRES_PASSWORD",
		dbEnv:    "POSTGRES_DB",
		dataPath: "/var/lib/postgresql/data",
		pgData:   "/var/lib/postgresql/data/pgdata",
	}
}

func (r *ManagedDatabaseReconciler) reconcileDeploymentDatabase(ctx context.Context, db *pb.ManagedDatabase) error {
	namespace := db.Namespace

	if err := r.reconcileDeploymentDatabaseNamespace(ctx, namespace); err != nil {
		return err
	}

	credentialsName := "openshell-db-credentials"
	if err := r.reconcileDeploymentDatabaseCredentials(ctx, namespace, credentialsName); err != nil {
		return err
	}

	dbImage := os.Getenv("OPENSHELL_DATABASE_IMAGE")
	if dbImage == "" {
		dbImage = "postgres:18"
	}
	postgresConfig := deploymentPostgresConfigForImage(dbImage)

	pvc := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "PersistentVolumeClaim",
			"metadata": map[string]interface{}{
				"name":      "openshell-gateway-db-data",
				"namespace": namespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":       "openshell",
					"app.kubernetes.io/component":  "database",
					"app.kubernetes.io/managed-by": "hypershell-control-plane",
					"hypershell.redhat.io/managed": "true",
				},
			},
			"spec": map[string]interface{}{
				"accessModes": []interface{}{"ReadWriteOnce"},
				"resources": map[string]interface{}{
					"requests": map[string]interface{}{
						"storage": "1Gi",
					},
				},
			},
		},
	}

	deployment := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "openshell-gateway-db",
				"namespace": namespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":       "openshell",
					"app.kubernetes.io/component":  "database",
					"app.kubernetes.io/managed-by": "hypershell-control-plane",
					"hypershell.redhat.io/managed": "true",
				},
			},
			"spec": map[string]interface{}{
				"replicas": int64(1),
				"strategy": map[string]interface{}{
					"type": "Recreate",
				},
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"app.kubernetes.io/name":     "openshell",
						"app.kubernetes.io/instance": "openshell-gateway-db",
					},
				},
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app.kubernetes.io/name":       "openshell",
							"app.kubernetes.io/instance":   "openshell-gateway-db",
							"app.kubernetes.io/component":  "database",
							"app.kubernetes.io/managed-by": "hypershell-control-plane",
							"hypershell.redhat.io/managed": "true",
						},
					},
					"spec": map[string]interface{}{
						"terminationGracePeriodSeconds": int64(30),
						"securityContext": map[string]interface{}{
							"runAsNonRoot":        true,
							"runAsUser":           postgresConfig.uid,
							"runAsGroup":          postgresConfig.uid,
							"fsGroup":             postgresConfig.uid,
							"fsGroupChangePolicy": "OnRootMismatch",
							"seccompProfile": map[string]interface{}{
								"type": "RuntimeDefault",
							},
						},
						"initContainers": []interface{}{
							map[string]interface{}{
								"name":            "prepare-postgres-run-directory",
								"image":           dbImage,
								"imagePullPolicy": "IfNotPresent",
								"command":         []interface{}{`/bin/sh`, `-ec`},
								"args":            []interface{}{`mkdir -p /work/postgresql && chmod 3775 /work/postgresql`},
								"securityContext": map[string]interface{}{
									"allowPrivilegeEscalation": false,
									"runAsNonRoot":             true,
									"runAsUser":                postgresConfig.uid,
									"runAsGroup":               postgresConfig.uid,
									"readOnlyRootFilesystem":   true,
									"capabilities": map[string]interface{}{
										"drop": []interface{}{"ALL"},
									},
								},
								"volumeMounts": []interface{}{
									map[string]interface{}{
										"name":      "postgres-run",
										"mountPath": "/work",
									},
								},
							},
						},
						"containers": []interface{}{
							map[string]interface{}{
								"name":            "postgresql",
								"image":           dbImage,
								"imagePullPolicy": "IfNotPresent",
								"securityContext": map[string]interface{}{
									"allowPrivilegeEscalation": false,
									"runAsNonRoot":             true,
									"runAsUser":                postgresConfig.uid,
									"runAsGroup":               postgresConfig.uid,
									"readOnlyRootFilesystem":   true,
									"seccompProfile": map[string]interface{}{
										"type": "RuntimeDefault",
									},
									"capabilities": map[string]interface{}{
										"drop": []interface{}{"ALL"},
									},
								},
								"env": []interface{}{
									map[string]interface{}{
										"name": postgresConfig.userEnv,
										"valueFrom": map[string]interface{}{
											"secretKeyRef": map[string]interface{}{
												"name": credentialsName,
												"key":  "user",
											},
										},
									},
									map[string]interface{}{
										"name": postgresConfig.passEnv,
										"valueFrom": map[string]interface{}{
											"secretKeyRef": map[string]interface{}{
												"name": credentialsName,
												"key":  "password",
											},
										},
									},
									map[string]interface{}{
										"name": postgresConfig.dbEnv,
										"valueFrom": map[string]interface{}{
											"secretKeyRef": map[string]interface{}{
												"name": credentialsName,
												"key":  "dbname",
											},
										},
									},
									map[string]interface{}{
										"name":  "PGDATA",
										"value": postgresConfig.pgData,
									},
								},
								"ports": []interface{}{
									map[string]interface{}{
										"name":          "postgresql",
										"containerPort": int64(5432),
										"protocol":      "TCP",
									},
								},
								"readinessProbe": map[string]interface{}{
									"tcpSocket": map[string]interface{}{
										"port": int64(5432),
									},
									"initialDelaySeconds": int64(5),
									"periodSeconds":       int64(10),
								},
								"livenessProbe": map[string]interface{}{
									"tcpSocket": map[string]interface{}{
										"port": int64(5432),
									},
									"initialDelaySeconds": int64(30),
									"periodSeconds":       int64(10),
								},
								"volumeMounts": []interface{}{
									map[string]interface{}{
										"name":      "db-data",
										"mountPath": postgresConfig.dataPath,
									},
									map[string]interface{}{
										"name":      "postgres-run",
										"mountPath": "/var/run/postgresql",
										"subPath":   "postgresql",
									},
									map[string]interface{}{
										"name":      "tmp",
										"mountPath": "/tmp",
									},
								},
								"resources": map[string]interface{}{
									"requests": map[string]interface{}{
										"cpu":    "100m",
										"memory": "256Mi",
									},
									"limits": map[string]interface{}{
										"cpu":    "500m",
										"memory": "512Mi",
									},
								},
							},
						},
						"volumes": []interface{}{
							map[string]interface{}{
								"name": "db-data",
								"persistentVolumeClaim": map[string]interface{}{
									"claimName": "openshell-gateway-db-data",
								},
							},
							map[string]interface{}{
								"name":     "postgres-run",
								"emptyDir": map[string]interface{}{},
							},
							map[string]interface{}{
								"name":     "tmp",
								"emptyDir": map[string]interface{}{},
							},
						},
					},
				},
			},
		},
	}

	if r.isOpenShift {
		deployment = stripOpenShiftPostgresSecurityContext(deployment)
	}
	svc := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      "openshell-gateway-db",
				"namespace": namespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":       "openshell",
					"app.kubernetes.io/component":  "database",
					"app.kubernetes.io/managed-by": "hypershell-control-plane",
					"hypershell.redhat.io/managed": "true",
				},
			},
			"spec": map[string]interface{}{
				"type": "ClusterIP",
				"ports": []interface{}{
					map[string]interface{}{
						"port":       int64(5432),
						"targetPort": "postgresql",
						"protocol":   "TCP",
						"name":       "postgresql",
					},
				},
				"selector": map[string]interface{}{
					"app.kubernetes.io/name":     "openshell",
					"app.kubernetes.io/instance": "openshell-gateway-db",
				},
			},
		},
	}

	for _, obj := range []*unstructured.Unstructured{pvc, svc, deployment} {
		if err := r.applyUnstructured(ctx, obj); err != nil {
			return fmt.Errorf("reconcile %s %s: %w", obj.GetKind(), obj.GetName(), err)
		}
	}

	if ready, _, err := gateway.DeploymentReadiness(ctx, r.clientset, namespace, "openshell-gateway-db"); err == nil && ready {
		return nil
	}

	deadline := time.After(2 * time.Minute)
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timed out waiting for deployment database to become ready in %s", namespace)
		case <-ticker.C:
			ready, _, err := gateway.DeploymentReadiness(ctx, r.clientset, namespace, "openshell-gateway-db")
			if err != nil {
				log.Printf("WARN error checking deployment database readiness in %s: %v", namespace, err)
				continue
			}
			if ready {
				log.Printf("INFO deployment database ready in %s", namespace)
				return nil
			}
		}
	}
}

var kindToGVR = map[string]schema.GroupVersionResource{
	"PersistentVolumeClaim": {Version: "v1", Resource: "persistentvolumeclaims"},
	"Service":               {Version: "v1", Resource: "services"},
	"Deployment":            {Group: "apps", Version: "v1", Resource: "deployments"},
}

func (r *ManagedDatabaseReconciler) applyUnstructured(ctx context.Context, obj *unstructured.Unstructured) error {
	gvr, ok := kindToGVR[obj.GetKind()]
	if !ok {
		return fmt.Errorf("unknown kind %q for applyUnstructured", obj.GetKind())
	}

	ns := obj.GetNamespace()
	name := obj.GetName()
	resourceClient := r.dynamicClient.Resource(gvr).Namespace(ns)

	existing, err := resourceClient.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return fmt.Errorf("get %s/%s: %w", obj.GetKind(), name, err)
		}
		if _, err := resourceClient.Create(ctx, obj, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create %s/%s: %w", obj.GetKind(), name, err)
		}
		log.Printf("INFO created %s %s in %s", obj.GetKind(), name, ns)
		return nil
	}

	var desired *unstructured.Unstructured
	switch obj.GetKind() {
	case "PersistentVolumeClaim":
		desired, err = convergeDeploymentDatabasePVC(existing, obj)
	case "Service":
		desired, err = convergeDeploymentDatabaseService(existing, obj)
	case "Deployment":
		desired, err = convergeDeploymentDatabaseDeployment(existing, obj)
	default:
		desired = obj.DeepCopy()
	}
	if err != nil {
		return fmt.Errorf("converge %s/%s: %w", obj.GetKind(), name, err)
	}

	if apiequality.Semantic.DeepDerivative(desired.Object, existing.Object) {
		log.Printf("DEBUG %s %s in %s already converged", obj.GetKind(), name, ns)
		return nil
	}

	desired.SetResourceVersion(existing.GetResourceVersion())
	if _, err := resourceClient.Update(ctx, desired, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update %s/%s: %w", obj.GetKind(), name, err)
	}
	log.Printf("INFO updated %s %s in %s", obj.GetKind(), name, ns)
	return nil
}

func convergeDeploymentDatabasePVC(existing, desired *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	merged := existing.DeepCopy()
	mergeDesiredLabels(merged, desired)

	existingModes, _, err := unstructured.NestedStringSlice(existing.Object, "spec", "accessModes")
	if err != nil {
		return nil, fmt.Errorf("read existing access modes: %w", err)
	}
	desiredModes, _, err := unstructured.NestedStringSlice(desired.Object, "spec", "accessModes")
	if err != nil {
		return nil, fmt.Errorf("read desired access modes: %w", err)
	}
	if !reflect.DeepEqual(existingModes, desiredModes) {
		return nil, fmt.Errorf("immutable accessModes drift: existing=%v desired=%v", existingModes, desiredModes)
	}

	existingStorage, _, err := unstructured.NestedString(existing.Object, "spec", "resources", "requests", "storage")
	if err != nil {
		return nil, fmt.Errorf("read existing storage request: %w", err)
	}
	desiredStorage, _, err := unstructured.NestedString(desired.Object, "spec", "resources", "requests", "storage")
	if err != nil {
		return nil, fmt.Errorf("read desired storage request: %w", err)
	}
	currentQuantity, err := resource.ParseQuantity(existingStorage)
	if err != nil {
		return nil, fmt.Errorf("parse existing storage request %q: %w", existingStorage, err)
	}
	desiredQuantity, err := resource.ParseQuantity(desiredStorage)
	if err != nil {
		return nil, fmt.Errorf("parse desired storage request %q: %w", desiredStorage, err)
	}
	if currentQuantity.Cmp(desiredQuantity) < 0 {
		if err := unstructured.SetNestedField(merged.Object, desiredStorage, "spec", "resources", "requests", "storage"); err != nil {
			return nil, fmt.Errorf("set storage request: %w", err)
		}
	}
	return merged, nil
}

func convergeDeploymentDatabaseService(existing, desired *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	merged := existing.DeepCopy()
	mergeDesiredLabels(merged, desired)
	for _, path := range [][]string{
		{"spec", "type"},
		{"spec", "selector"},
		{"spec", "ports"},
	} {
		value, found, err := unstructured.NestedFieldCopy(desired.Object, path...)
		if err != nil {
			return nil, fmt.Errorf("read desired field %s: %w", strings.Join(path, "."), err)
		}
		if !found {
			unstructured.RemoveNestedField(merged.Object, path...)
			continue
		}
		if err := unstructured.SetNestedField(merged.Object, value, path...); err != nil {
			return nil, fmt.Errorf("set desired field %s: %w", strings.Join(path, "."), err)
		}
	}
	return merged, nil
}

func convergeDeploymentDatabaseDeployment(existing, desired *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	existingSelector, _, err := unstructured.NestedMap(existing.Object, "spec", "selector")
	if err != nil {
		return nil, fmt.Errorf("read existing selector: %w", err)
	}
	desiredSelector, _, err := unstructured.NestedMap(desired.Object, "spec", "selector")
	if err != nil {
		return nil, fmt.Errorf("read desired selector: %w", err)
	}
	if !reflect.DeepEqual(existingSelector, desiredSelector) {
		return nil, fmt.Errorf("immutable selector drift: existing=%v desired=%v", existingSelector, desiredSelector)
	}
	return desired.DeepCopy(), nil
}

func mergeDesiredLabels(existing, desired *unstructured.Unstructured) {
	labels := existing.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	for key, value := range desired.GetLabels() {
		labels[key] = value
	}
	existing.SetLabels(labels)
}

// stripOpenShiftPostgresSecurityContext removes fixed identities so OpenShift SCC can assign its range.
func stripOpenShiftPostgresSecurityContext(deployment *unstructured.Unstructured) *unstructured.Unstructured {
	if deployment == nil {
		return nil
	}
	stripped := deployment.DeepCopy()
	if securityContext, found, _ := unstructured.NestedMap(stripped.Object, "spec", "template", "spec", "securityContext"); found {
		for _, field := range []string{"runAsUser", "runAsGroup", "fsGroup", "fsGroupChangePolicy"} {
			delete(securityContext, field)
		}
		_ = unstructured.SetNestedMap(stripped.Object, securityContext, "spec", "template", "spec", "securityContext")
	}
	for _, containerField := range []string{"initContainers", "containers"} {
		containers, found, err := unstructured.NestedSlice(stripped.Object, "spec", "template", "spec", containerField)
		if err != nil || !found {
			continue
		}
		for i, container := range containers {
			containerMap, ok := container.(map[string]interface{})
			if !ok {
				continue
			}
			securityContext, ok := containerMap["securityContext"].(map[string]interface{})
			if !ok {
				continue
			}
			delete(securityContext, "runAsUser")
			delete(securityContext, "runAsGroup")
			containerMap["securityContext"] = securityContext
			containers[i] = containerMap
		}
		_ = unstructured.SetNestedSlice(stripped.Object, containers, "spec", "template", "spec", containerField)
	}
	return stripped
}
func (r *ManagedDatabaseReconciler) deleteDeploymentDatabase(ctx context.Context, namespace string) error {
	resources := []struct {
		gvr  schema.GroupVersionResource
		name string
	}{
		{schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, "openshell-gateway-db"},
		{schema.GroupVersionResource{Version: "v1", Resource: "services"}, "openshell-gateway-db"},
		{schema.GroupVersionResource{Version: "v1", Resource: "persistentvolumeclaims"}, "openshell-gateway-db-data"},
		{schema.GroupVersionResource{Version: "v1", Resource: "secrets"}, "openshell-db-credentials"},
	}

	var errs []error
	for _, res := range resources {
		if err := r.dynamicClient.Resource(res.gvr).Namespace(namespace).Delete(ctx, res.name, metav1.DeleteOptions{}); err != nil {
			if !k8serrors.IsNotFound(err) {
				errs = append(errs, fmt.Errorf("delete %s %s in %s: %w", res.gvr.Resource, res.name, namespace, err))
			}
		} else {
			log.Printf("INFO deleted %s %s from %s", res.gvr.Resource, res.name, namespace)
		}
	}

	if err := r.clientset.CoreV1().Namespaces().Delete(ctx, namespace, metav1.DeleteOptions{}); err != nil {
		if !k8serrors.IsNotFound(err) {
			errs = append(errs, fmt.Errorf("delete namespace %s: %w", namespace, err))
		}
	} else {
		log.Printf("INFO deleted namespace %s", namespace)
	}
	return errors.Join(errs...)
}

func (r *ManagedDatabaseReconciler) updateManagedDatabaseStatusIfChanged(ctx context.Context, id, current, desired string) {
	if current == desired {
		return
	}
	r.updateManagedDatabaseStatus(ctx, id, desired)
}

func (r *ManagedDatabaseReconciler) updateManagedDatabaseStatus(ctx context.Context, id, status string) {
	client := pb.NewManagedDatabaseServiceClient(r.grpcConn)
	_, err := client.UpdateManagedDatabase(ctx, &pb.UpdateManagedDatabaseRequest{
		Id:     id,
		Status: &status,
	})
	if err != nil {
		log.Printf("WARN failed to update ManagedDatabase %s status to %s: %v", id, status, err)
	}
}

func cnpgClusterGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    "postgresql.cnpg.io",
		Version:  "v1",
		Resource: "clusters",
	}
}

type GatewayReleaseReconciler struct {
	mu     sync.Mutex
	active map[string]struct{}
}

func NewGatewayReleaseReconciler() *GatewayReleaseReconciler {
	return &GatewayReleaseReconciler{active: make(map[string]struct{})}
}

func (r *GatewayReleaseReconciler) Handle(ctx context.Context, event watcher.Event[*pb.GatewayRelease]) error {
	r.mu.Lock()
	if _, ok := r.active[event.ResourceID]; ok {
		r.mu.Unlock()
		return nil
	}
	r.active[event.ResourceID] = struct{}{}
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.active, event.ResourceID)
		r.mu.Unlock()
	}()

	_, endSpan := cpotel.StartReconcileSpan(ctx, "GatewayRelease", event.Type.String())
	defer func() { endSpan(nil) }()

	log.Printf("INFO reconciling GatewayRelease %s (event=%d)", event.ResourceID, event.Type)
	return nil
}

type GatewayReconciler struct {
	mu                    sync.Mutex
	active                map[string]struct{}
	dynamicClient         dynamic.Interface
	clientset             *kubernetes.Clientset
	grpcConn              *grpc.ClientConn
	manifests             map[string][]*unstructured.Unstructured
	isOpenShift           bool
	hasCertManager        bool
	hasGatewayAPI         bool
	ingressMode           string
	skipNetworkPolicies   bool
	hasCNPG               bool
	manifestsDir          string
	controlPlaneNamespace string
	keycloakClient        *keycloak.Client
	keycloakConfig        *gateway.KeycloakConfig
	exposure              exposure.Port
}

func NewGatewayReconciler(
	dynamicClient dynamic.Interface,
	clientset *kubernetes.Clientset,
	grpcConn *grpc.ClientConn,
	manifestsDir string,
	controlPlaneNamespace string,
	keycloakConfig *gateway.KeycloakConfig,
	exposurePort exposure.Port,
) (*GatewayReconciler, error) {
	manifests, err := gateway.LoadGatewayManifests(manifestsDir)
	if err != nil {
		return nil, fmt.Errorf("load gateway manifests from %s: %w", manifestsDir, err)
	}

	isOpenShift := gateway.DetectOpenShift(clientset)
	hasCertManager := gateway.DetectCertManager(clientset)
	hasGatewayAPI := gateway.DetectGatewayAPI(clientset)
	ingressMode := gateway.IngressMode(hasGatewayAPI, isOpenShift)
	skipNetworkPolicies := os.Getenv("GATEWAY_SKIP_NETWORK_POLICIES") == "true"
	hasCNPG := gateway.DetectCNPG(clientset)

	var kcClient *keycloak.Client
	if keycloakConfig != nil {
		kcClient = keycloak.NewClient(
			keycloakConfig.ServerURL,
			keycloakConfig.Realm,
			keycloakConfig.ClientID,
			keycloakConfig.ClientSecret,
		)
		log.Printf("INFO keycloak integration enabled: server=%s realm=%s", keycloakConfig.ServerURL, keycloakConfig.Realm)
	}

	log.Printf("INFO gateway reconciler initialized: manifests=%d openshift=%v certmanager=%v gatewayapi=%v ingressMode=%s cnpg=%v keycloak=%v netpol=%v",
		len(manifests), isOpenShift, hasCertManager, hasGatewayAPI, ingressMode, hasCNPG, kcClient != nil, !skipNetworkPolicies)

	return &GatewayReconciler{
		active:                make(map[string]struct{}),
		dynamicClient:         dynamicClient,
		clientset:             clientset,
		grpcConn:              grpcConn,
		manifests:             manifests,
		isOpenShift:           isOpenShift,
		hasCertManager:        hasCertManager,
		hasGatewayAPI:         hasGatewayAPI,
		ingressMode:           ingressMode,
		skipNetworkPolicies:   skipNetworkPolicies,
		hasCNPG:               hasCNPG,
		manifestsDir:          manifestsDir,
		controlPlaneNamespace: controlPlaneNamespace,
		keycloakClient:        kcClient,
		keycloakConfig:        keycloakConfig,
		exposure:              exposurePort,
	}, nil
}

func (r *GatewayReconciler) Handle(ctx context.Context, event watcher.Event[*pb.Gateway]) error {
	r.mu.Lock()
	if _, ok := r.active[event.ResourceID]; ok {
		r.mu.Unlock()
		return nil
	}
	r.active[event.ResourceID] = struct{}{}
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.active, event.ResourceID)
		r.mu.Unlock()
	}()

	gw := event.Resource
	if gw == nil {
		log.Printf("WARN gateway event %s has nil resource, skipping", event.ResourceID)
		return nil
	}
	previousPhase := gw.GetPhase()
	if event.PhaseBeforeRetry != "" {
		previousPhase = event.PhaseBeforeRetry
	}
	suppressGatewayProvisionObservation(event.ResourceID, previousPhase)

	ctx, endSpan := cpotel.StartReconcileSpan(ctx, "Gateway", event.Type.String())
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("hypershell.resource_id", event.ResourceID))
	var reconcileErr error
	defer func() { endSpan(reconcileErr) }()

	if event.Type == watcher.EventDeleted {
		forgetGatewayProvisionObservation(event.ResourceID)
		var deleteDBConfig databaseConfig
		var deleteErrs []error
		if gw.DatabaseId != "" {
			var dbErr error
			deleteDBConfig, dbErr = r.resolveDatabaseConfig(ctx, gw)
			if dbErr != nil {
				// Deletion must be idempotent. When the ManagedDatabase is already
				// gone (a legitimate delete ordering) there is no DB config left to
				// resolve and nothing more to tear down for it, so treat NotFound as
				// already-cleaned and continue finalizing the gateway. Appending it to
				// deleteErrs instead would fail the whole delete-reconcile, which the
				// watcher retries every 30s -- forever, because the ManagedDatabase
				// never comes back. Any other error still fails so it is retried.
				if status.Code(dbErr) == codes.NotFound {
					log.Printf("INFO gateway %s: ManagedDatabase %s already deleted; skipping database cleanup", event.ResourceID, gw.DatabaseId)
				} else {
					deleteErrs = append(deleteErrs, fmt.Errorf("resolve database config for deleted gateway %s: %w", event.ResourceID, dbErr))
				}
			}
		}

		namespace, namespaceErr := gatewayNamespace(gw)
		if namespaceErr != nil {
			// Without a recorded namespace there is nothing deterministic to clean
			// up. Continue with ManagedDatabase deletion rather than leaking a
			// dedicated deployment database; NamespaceGC remains the namespace
			// backstop.
			log.Printf("WARN gateway %s deleted but %v; skipping namespace cleanup", event.ResourceID, namespaceErr)
		} else {
			log.Printf("INFO gateway %s deleted, cleaning up resources in namespace %s", event.ResourceID, namespace)
			opts := gateway.ReconcileOpts{
				IsOpenShift:           r.isOpenShift,
				HasCertManager:        r.hasCertManager,
				HasGatewayAPI:         r.hasGatewayAPI,
				SkipNetworkPolicies:   r.skipNetworkPolicies,
				HasCNPG:               r.hasCNPG,
				CNPG:                  deleteDBConfig.CNPG,
				DatabaseProvider:      deleteDBConfig.Provider,
				DeploymentDBNamespace: deleteDBConfig.SourceNamespace,
				ControlPlaneNamespace: r.controlPlaneNamespace,
				KeycloakClient:        r.keycloakClient,
				GatewayID:             event.ResourceID,
				GatewayName:           gw.Name,
			}
			var credentialNamespaces []string
			if gw.CredentialDriver != nil && *gw.CredentialDriver != "" {
				if strings.Contains(*gw.CredentialDriver, "kubernetes_secrets") {
					var credCfg gateway.CredentialDriverConfig
					if err := json.Unmarshal([]byte(*gw.CredentialDriver), &credCfg); err == nil {
						if credCfg.KubernetesSecrets != nil && credCfg.KubernetesSecrets.Namespace != "" {
							credentialNamespaces = append(credentialNamespaces, credCfg.KubernetesSecrets.Namespace)
						}
					}
				}
			}
			if err := gateway.DeleteGatewayResources(ctx, r.dynamicClient, r.clientset, namespace, opts, credentialNamespaces...); err != nil {
				deleteErrs = append(deleteErrs, fmt.Errorf("delete gateway resources in %s: %w", namespace, err))
			} else {
				log.Printf("INFO gateway %s resources cleaned up from namespace %s", event.ResourceID, namespace)
			}

			// Namespace deletion and database deletion are independent cleanup
			// operations. Attempt both and aggregate failures so one partial failure
			// cannot silently leak the other resource.
			deleted, err := gateway.DeleteManagedNamespace(ctx, r.clientset, namespace, r.controlPlaneNamespace)
			if err != nil {
				deleteErrs = append(deleteErrs, fmt.Errorf("delete gateway namespace %s: %w", namespace, err))
			} else if !deleted {
				gateway.DeleteLabeledNamespaceResources(ctx, r.dynamicClient, namespace, opts)
			}
		}

		if deleteDBConfig.Provider == "deployment" && gw.DatabaseId != "" {
			client := pb.NewManagedDatabaseServiceClient(r.grpcConn)
			if err := deleteGatewayDeploymentDatabase(ctx, client, gw.DatabaseId); err != nil {
				deleteErrs = append(deleteErrs, fmt.Errorf("delete deployment ManagedDatabase %s for gateway %s: %w", gw.DatabaseId, event.ResourceID, err))
			} else {
				log.Printf("INFO deleted deployment ManagedDatabase %s for gateway %s", gw.DatabaseId, event.ResourceID)
			}
		}
		reconcileErr = errors.Join(deleteErrs...)
		return reconcileErr
	}

	log.Printf("INFO reconciling Gateway %s name=%q namespace=%s (event=%d)",
		event.ResourceID, gw.Name, gw.Namespace, event.Type)

	// The phase gate prevents redundant re-provisioning (re-applying manifests)
	// of a Gateway that has already been acted upon. Running, Provisioning, and
	// Degraded gateways are owned by the continuous health reconciler, which
	// keeps their phase synchronized with workload health via a separate path
	// that this gate does not suppress. See openshell-gateway-health.spec.md.
	//
	// Keycloak client attributes are external desired state, however, and need a
	// lightweight drift reconciliation even when Kubernetes reprovisioning is
	// gated. In particular, controller startup seeds existing Running gateways;
	// reconciling before the return below lets newly introduced client settings
	// converge without forcing a full gateway rollout.
	if gw.Phase != nil && (*gw.Phase == gatewayPhaseRunning || *gw.Phase == gatewayPhaseProvisioning || *gw.Phase == gatewayPhaseDegraded) {
		if err := r.reconcileExistingGatewayKeycloakClient(ctx, event.ResourceID, gw); err != nil {
			var identityErr *gatewayKeycloakClientIdentityError
			if errors.As(err, &identityErr) {
				// Invalid persisted identity is terminal desired-state validation, not a
				// transient Keycloak outage. Publish a fixed marker once and stop retrying;
				// retry only when the status write itself fails.
				if gw.GetStatus() == gatewayKeycloakClientInvalidStatus {
					return nil
				}
				if statusErr := r.updateGatewayStatus(ctx, event.ResourceID, gatewayKeycloakClientInvalidStatus); statusErr != nil {
					return watcher.PreservePayloadForRetry(errors.Join(
						fmt.Errorf("validate existing Keycloak client identity: %w", err),
						fmt.Errorf("publish invalid Keycloak client configuration status: %w", statusErr),
					))
				}
				return nil
			}
			if errors.Is(err, errGatewayKeycloakClientMissing) && gw.GetStatus() != gatewayKeycloakClientMissingStatus {
				if statusErr := r.updateGatewayStatus(ctx, event.ResourceID, gatewayKeycloakClientMissingStatus); statusErr != nil {
					err = errors.Join(err, fmt.Errorf("publish missing Keycloak client status: %w", statusErr))
				}
			}
			// This work is intentionally narrower than gateway provisioning. Keep the
			// gated payload on retry so the queue does not clear phase and expand a
			// transient Keycloak failure into a full Kubernetes reconciliation. The
			// fixed status write above generates another watch event, but the queue's
			// per-key backoff floor prevents that self-event from creating a hot loop.
			return watcher.PreservePayloadForRetry(fmt.Errorf("reconcile Keycloak client for gateway %q: %w", gw.Name, err))
		}
		if r.keycloakClient != nil && isGatewayKeycloakClientStatus(gw.GetStatus()) {
			if err := r.updateGatewayStatus(ctx, event.ResourceID, ""); err != nil {
				return watcher.PreservePayloadForRetry(fmt.Errorf("clear Keycloak client status for gateway %q: %w", gw.Name, err))
			}
		}
		log.Printf("DEBUG gateway %s phase=%s, skipping full reconciliation", event.ResourceID, *gw.Phase)
		return nil
	}

	var dbConfig databaseConfig
	if gw.DatabaseId != "" {
		var resolveErr error
		dbConfig, resolveErr = r.resolveDatabaseConfig(ctx, gw)
		if resolveErr != nil {
			reconcileErr = fmt.Errorf("resolve database config for gateway %s: %w", gw.Name, resolveErr)
			return reconcileErr
		}
	} else {
		log.Printf("INFO gateway %s has no database_id; skipping database reconciliation (existing database resources left untouched)", event.ResourceID)
	}

	namespace, err := gatewayNamespace(gw)
	if err != nil {
		reconcileErr = fmt.Errorf("reconcile gateway %s: %w", gw.Name, err)
		return reconcileErr
	}

	dnsNames := gw.ServerDnsNames
	if len(dnsNames) == 0 {
		dnsNames = []string{
			fmt.Sprintf("openshell-gateway.%s.svc.cluster.local", namespace),
		}
		if gw.ExternalDns != nil && *gw.ExternalDns != "" {
			dnsNames = append(dnsNames, *gw.ExternalDns)
		}
	}

	externalDns := ""
	if gw.ExternalDns != nil {
		externalDns = *gw.ExternalDns
	}

	gwConfig := gateway.GatewayConfig{
		ServerDnsNames: dnsNames,
		ExternalDns:    externalDns,
	}

	if gw.Image != nil && *gw.Image != "" {
		gwConfig.Image = *gw.Image
	}

	if gw.SupervisorImage != nil && *gw.SupervisorImage != "" {
		gwConfig.SupervisorImage = *gw.SupervisorImage
	}

	if gw.Oidc != nil && *gw.Oidc != "" {
		var oidcConfig gateway.OIDCConfig
		if err := json.Unmarshal([]byte(*gw.Oidc), &oidcConfig); err != nil {
			reconcileErr = fmt.Errorf("invalid oidc config for gateway %s: %w", gw.Name, err)
			return reconcileErr
		}
		gwConfig.OIDC = oidcConfig
	}

	if gw.Route != nil {
		routeConfig, err := parseGatewayRouteConfig(*gw.Route)
		if err != nil {
			reconcileErr = fmt.Errorf("invalid route config for gateway %s: %w", gw.Name, err)
			return reconcileErr
		}
		gwConfig.Route = routeConfig
	}

	if gw.CredentialDriver != nil && *gw.CredentialDriver != "" {
		var credDriverConfig gateway.CredentialDriverConfig
		decoder := json.NewDecoder(bytes.NewReader([]byte(*gw.CredentialDriver)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&credDriverConfig); err != nil {
			reconcileErr = fmt.Errorf("invalid credential driver config for gateway %s: %w", gw.Name, err)
			return reconcileErr
		}
		gwConfig.CredentialDriver = &credDriverConfig
	}

	nsConfig := gateway.NamespaceConfig{
		Name:    namespace,
		Gateway: gwConfig,
	}

	opts := gateway.ReconcileOpts{
		IsOpenShift:           r.isOpenShift,
		HasCertManager:        r.hasCertManager,
		HasGatewayAPI:         r.hasGatewayAPI,
		SkipNetworkPolicies:   r.skipNetworkPolicies,
		HasCNPG:               r.hasCNPG,
		DatabaseProvider:      dbConfig.Provider,
		CNPG:                  dbConfig.CNPG,
		DeploymentDBNamespace: dbConfig.SourceNamespace,
		ControlPlaneNamespace: r.controlPlaneNamespace,
		GatewayID:             event.ResourceID,
		UpdateRouteAddress:    r.makeRouteAddressUpdater(event.ResourceID),
		UpdateConsoleAddress:  r.makeConsoleAddressUpdater(event.ResourceID),
		Keycloak:              r.keycloakConfig,
		KeycloakClient:        r.keycloakClient,
		GatewayName:           gw.Name,
		UpdateOIDC:            r.makeOIDCUpdater(event.ResourceID),
		Exposure:              r.exposure,
		RouteStillDesired:     r.makeRouteStillDesired(event.ResourceID),
	}

	r.updateGatewayPhase(ctx, event.ResourceID, gatewayPhaseProvisioning)

	if err := gateway.ReconcileGateway(ctx, r.dynamicClient, r.clientset, nsConfig, r.manifests, opts); err != nil {
		r.updateGatewayPhase(ctx, event.ResourceID, gatewayPhaseFailed)
		reconcileErr = fmt.Errorf("reconcile gateway %s: %w", gw.Name, err)
		return reconcileErr
	}

	// Manifests are applied, but the gateway is not Running until its workload is
	// observed Ready. Wait within the provisioning readiness window; if the
	// Deployment never becomes ready, set Degraded and record why.
	ready, reason := gateway.WaitForGatewayReady(ctx, r.clientset, namespace, 2*time.Minute)
	if !ready {
		r.updateGatewayHealth(ctx, event.ResourceID, gatewayPhaseDegraded, reason)
		log.Printf("WARN gateway %s applied but not ready in namespace %s: %s", gw.Name, namespace, reason)
		return nil
	}

	// The Deployment is Ready. A routed gateway is not Running until its external
	// exposure is also observed Ready. Poll the exposure here within a bounded
	// window so the gateway is promoted to Running promptly once its route is
	// programmed - rather than waiting up to a full health-reconciler tick, which
	// would leave the connection command and console button hidden for seconds
	// after the pods are ready. If the window elapses, park at Provisioning and
	// let the continuous health reconciler keep enforcing the full route-readiness
	// grace window (promoting to Running, or Degraded once it expires). A
	// non-routed gateway - or any gateway on a cluster without the exposure port -
	// is Running on Deployment readiness alone. See
	// openshell-gateway-health.spec.md § Phase Reflects Workload and Route Readiness.
	routed := isRoutedGateway(gw)
	if r.exposure != nil && routed {
		if r.waitForRouteReady(ctx, namespace) {
			// The observation guard rejects work that started in Running or Degraded.
			if runningGateway := r.updateGatewayHealth(ctx, event.ResourceID, gatewayPhaseRunning, gatewayStatusHealthy); runningGateway != nil {
				observeGatewayProvisionDuration(ctx, runningGateway)
			}
			log.Printf("INFO gateway %s provisioned and route ready in namespace %s", gw.Name, namespace)
		} else {
			r.updateGatewayHealth(ctx, event.ResourceID, gatewayPhaseProvisioning, "Deployment ready; awaiting route readiness")
			log.Printf("INFO gateway %s deployment ready in namespace %s; awaiting route readiness", gw.Name, namespace)
		}
	} else {
		// The observation guard rejects work that started in Running or Degraded.
		if runningGateway := r.updateGatewayHealth(ctx, event.ResourceID, gatewayPhaseRunning, gatewayStatusHealthy); runningGateway != nil {
			observeGatewayProvisionDuration(ctx, runningGateway)
		}
		log.Printf("INFO gateway %s provisioned and ready in namespace %s", gw.Name, namespace)
	}

	// The console can start after the gateway is ready. Poll the console in the
	// background so the address appears without waiting for the next health tick.
	// The health reconciler continues to publish and retract the address.
	if routed && r.ingressMode != gateway.IngressModeNone {
		go r.publishConsoleAddressWhenReady(ctx, event.ResourceID, gw)
	}
	return nil
}

const (
	gatewayKeycloakClientMissingStatus = "Keycloak client is missing"
	gatewayKeycloakClientInvalidStatus = "Keycloak client configuration is invalid"
)

var errGatewayKeycloakClientMissing = errors.New("gateway Keycloak client is missing")

type gatewayKeycloakClientIdentityError struct {
	err error
}

func (e *gatewayKeycloakClientIdentityError) Error() string { return e.err.Error() }
func (e *gatewayKeycloakClientIdentityError) Unwrap() error { return e.err }

func invalidGatewayKeycloakClientIdentity(format string, args ...any) error {
	return &gatewayKeycloakClientIdentityError{err: fmt.Errorf(format, args...)}
}

func isGatewayKeycloakClientStatus(status string) bool {
	return status == gatewayKeycloakClientMissingStatus || status == gatewayKeycloakClientInvalidStatus
}

// reconcileExistingGatewayKeycloakClient reconciles the desired Keycloak client
// without reapplying the gateway's Kubernetes resources. External client
// attributes can drift or be introduced in newer controller releases. A missing
// client is reported as non-converged rather than silently skipped or partially
// recreated without restoring its user role assignments.
func (r *GatewayReconciler) reconcileExistingGatewayKeycloakClient(ctx context.Context, gatewayID string, gw *pb.Gateway) error {
	if r.keycloakClient == nil {
		return nil
	}
	clientID, err := existingGatewayKeycloakClientID(gatewayID, gw)
	if err != nil {
		return err
	}

	clientUUID, err := r.keycloakClient.GetClientUUID(ctx, clientID)
	if err != nil {
		return fmt.Errorf("check existing Keycloak client %q: %w", clientID, err)
	}
	if clientUUID == "" {
		return fmt.Errorf("desired Keycloak client %q is missing: %w", clientID, errGatewayKeycloakClientMissing)
	}
	if err := r.keycloakClient.EnsureDeviceAuthorizationGrant(ctx, clientUUID); err != nil {
		return fmt.Errorf("reconcile device authorization grant on Keycloak client %q: %w", clientID, err)
	}
	log.Printf("INFO reconciled Keycloak client %q (uuid=%q)", clientID, clientUUID)
	return nil
}

// existingGatewayKeycloakClientID returns the identity recorded when the client
// was provisioned. Gateway names are mutable, so recomputing from the current name
// can miss the real client after a rename. client_id is present on newer rows;
// audience carries the same value on legacy rows. Persisted values cross an API
// trust boundary, so they are accepted only when they agree, contain no control
// characters, and match a gateway-owned historical identity format: either the
// gateway ID itself or a prefix followed by "-<gatewayID>". The prefix is not
// otherwise restricted because historical clients were provisioned from the raw
// user-visible gateway name. Only gateways without either persisted value fall
// back to the provisioning format based on the current name.
func existingGatewayKeycloakClientID(gatewayID string, gw *pb.Gateway) (string, error) {
	if gatewayID == "" {
		return "", invalidGatewayKeycloakClientIdentity("gateway ID is required for Keycloak reconciliation")
	}
	if containsControlCharacter(gatewayID) {
		return "", invalidGatewayKeycloakClientIdentity("gateway ID contains control characters")
	}
	if gw == nil {
		return "", invalidGatewayKeycloakClientIdentity("gateway is required for Keycloak reconciliation")
	}

	var clientID, audience string
	if gw.GetOidc() != "" {
		var oidc gateway.OIDCConfig
		if err := json.Unmarshal([]byte(gw.GetOidc()), &oidc); err != nil {
			return "", invalidGatewayKeycloakClientIdentity("parse persisted OIDC config: %w", err)
		}
		clientID, audience = oidc.ClientID, oidc.Audience
	}

	for _, persisted := range []string{clientID, audience} {
		if persisted != "" && containsControlCharacter(persisted) {
			return "", invalidGatewayKeycloakClientIdentity("persisted OIDC client identity contains control characters")
		}
	}
	if clientID != "" && audience != "" && clientID != audience {
		return "", invalidGatewayKeycloakClientIdentity("persisted OIDC client_id and audience do not match")
	}

	persisted := clientID
	if persisted == "" {
		persisted = audience
	}
	if persisted != "" {
		if !isGatewayOwnedKeycloakClientID(persisted, gatewayID) {
			return "", invalidGatewayKeycloakClientIdentity("persisted OIDC client identity is not owned by the gateway")
		}
		return persisted, nil
	}

	if gw.GetName() == "" {
		return "", invalidGatewayKeycloakClientIdentity("gateway name is required for Keycloak reconciliation")
	}
	fallback := fmt.Sprintf("%s-%s", gw.GetName(), gatewayID)
	if containsControlCharacter(fallback) {
		return "", invalidGatewayKeycloakClientIdentity("gateway-derived Keycloak client identity contains control characters")
	}
	if !isGatewayOwnedKeycloakClientID(fallback, gatewayID) {
		return "", invalidGatewayKeycloakClientIdentity("gateway-derived Keycloak client identity is not owned by the gateway")
	}
	return fallback, nil
}

func isGatewayOwnedKeycloakClientID(clientID, gatewayID string) bool {
	if clientID == "" || containsControlCharacter(clientID) {
		return false
	}
	if clientID == gatewayID {
		return true
	}
	suffix := "-" + gatewayID
	return strings.HasSuffix(clientID, suffix) && len(clientID) > len(suffix)
}

func containsControlCharacter(value string) bool {
	for _, ch := range value {
		if unicode.IsControl(ch) {
			return true
		}
	}
	return false
}

// provisioningRouteReadyWait bounds how long the provisioning path polls a
// routed gateway's external exposure for readiness before parking it at
// Provisioning. Route programming typically completes within a few seconds of
// Deployment readiness; polling here (rather than waiting for the health loop's
// next tick) lets the connection command and console surface promptly. On
// timeout the health reconciler continues enforcing the full route-readiness
// grace window, so a slow route is not misreported.
const provisioningRouteReadyWait = 90 * time.Second

// provisioningRouteReadyInterval is the cadence at which the provisioning path
// polls a routed gateway's exposure. It is intentionally far tighter than the
// steady-state health interval (30s) so the first Running promotion is prompt;
// the 30s cadence still governs ongoing health once the gateway is settled.
const provisioningRouteReadyInterval = 2 * time.Second

// provisioningConsoleReadyWait bounds how long the provisioning path polls a
// routed gateway's console Deployment for readiness before leaving further
// publication to the health reconciler. It is generous because the console
// images may need pulling on a cold cluster; the poll runs in the background,
// so a long window never blocks the gateway watch loop.
const provisioningConsoleReadyWait = 5 * time.Minute

// provisioningConsoleReadyInterval is the cadence at which the provisioning path
// polls the console Deployment's readiness, tight enough that the console button
// enables within a couple of seconds of the pod becoming ready.
const provisioningConsoleReadyInterval = 2 * time.Second

// waitForRouteReady polls the gateway's external exposure until it reports Ready
// or the bounded provisioning window elapses, returning whether it became Ready.
func (r *GatewayReconciler) waitForRouteReady(ctx context.Context, namespace string) bool {
	return r.pollRouteReady(ctx, namespace, provisioningRouteReadyInterval, provisioningRouteReadyWait)
}

// pollRouteReady observes the exposure immediately and then every interval until
// it reports Ready or the window elapses, mirroring WaitForGatewayReady so a
// route that is already (or quickly) programmed promotes without waiting a full
// interval. A transient observation error is logged and retried, never treated
// as not-ready-forever. Interval and window are parameters so tests can drive it
// without real-time waits.
func (r *GatewayReconciler) pollRouteReady(ctx context.Context, namespace string, interval, window time.Duration) bool {
	return poll(ctx, interval, window, func() bool {
		rr, err := r.exposure.ObserveReadiness(ctx, exposure.Request{Namespace: namespace})
		if err != nil {
			log.Printf("WARN gateway route readiness for %s: %v", namespace, err)
			return false
		}
		return rr.Ready
	})
}

// publishConsoleAddressWhenReady polls the gateway's console Deployment on a
// tight cadence and publishes console_address as soon as the console pod can
// serve, so the web UI's console button enables promptly rather than waiting for
// the next health-reconciler tick. It is meant to run in the background and
// stops once the address is published or the bounded window elapses.
//
// It runs on the long-lived watch context, which route removal does not cancel,
// so it must not trust the routed Gateway snapshot captured when provisioning
// started. If the route is removed while the console image is still pulling, the
// health reconciler's teardown owns clearing console_address; a publisher acting
// on the stale snapshot would otherwise re-publish the console link after
// teardown, stranding a trusted address for a gateway that is no longer routed.
// Re-read the current Gateway each poll and stop the moment it is no longer
// routed (or has been deleted), leaving the address to teardown.
func (r *GatewayReconciler) publishConsoleAddressWhenReady(ctx context.Context, gatewayID string, gw *pb.Gateway) {
	client := pb.NewGatewayServiceClient(r.grpcConn)
	poll(ctx, provisioningConsoleReadyInterval, provisioningConsoleReadyWait, func() bool {
		resp, err := client.GetGateway(ctx, &pb.GetGatewayRequest{Id: gatewayID})
		if err != nil {
			if status.Code(err) == codes.NotFound {
				// The gateway was deleted while the console image pulled: there is
				// nothing left to publish against. Stop polling rather than retrying
				// a NotFound until the window elapses.
				return true
			}
			log.Printf("WARN console publisher: get gateway %s: %v", gatewayID, err)
			return false
		}
		current := resp.GetGateway()
		if current == nil || !isRoutedGateway(current) {
			// No longer routed (or gone): teardown owns the console_address now.
			// End the poll rather than publishing against the stale snapshot.
			return true
		}
		return syncConsoleAddress(ctx, r.clientset, r.dynamicClient, client, gatewayID, current, r.ingressMode)
	})
}

// makeRouteStillDesired returns a callback the provisioning path invokes after
// the (up-to-60s) server-TLS wait, before it creates the remaining route- and
// console-owned resources, to confirm the gateway is still routed according to
// its live API-server record. A route removal (or gateway deletion) during that
// wait is observed only by the independent health loop -- the watcher phase gate
// blocks a re-provision -- which tears the gateway down and clears its stored
// addresses; without this re-check the in-flight pass would recreate the
// BackendTLSPolicy, backend-CA ConfigMap, router NetworkPolicy, console, and
// Keycloak client behind that teardown, and the health loop's torn-down cache
// (keyed on empty addresses) would then hide the orphans indefinitely. Returns
// false on NotFound (the gateway is gone, so nothing is desired) and propagates
// transient errors so the caller can decide (it proceeds conservatively).
func (r *GatewayReconciler) makeRouteStillDesired(gatewayID string) func(context.Context) (bool, error) {
	return func(ctx context.Context) (bool, error) {
		client := pb.NewGatewayServiceClient(r.grpcConn)
		resp, err := client.GetGateway(ctx, &pb.GetGatewayRequest{Id: gatewayID})
		if err != nil {
			if status.Code(err) == codes.NotFound {
				return false, nil
			}
			return false, err
		}
		return isRoutedGateway(resp.GetGateway()), nil
	}
}

// poll invokes attempt immediately and then every interval until it returns
// true or the window elapses (or the context is cancelled), reporting whether
// attempt ever succeeded. Interval and window are parameters so tests can drive
// it without real-time waits.
func poll(ctx context.Context, interval, window time.Duration, attempt func() bool) bool {
	deadline := time.After(window)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if attempt() {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-deadline:
			return false
		case <-ticker.C:
		}
	}
}

// parseGatewayRouteConfig parses the route field. A route object is enabled by
// default for compatibility with existing empty and host-only route objects.
// An explicit enabled=false value disables it. An empty or null value has no
// route.
func parseGatewayRouteConfig(raw string) (gateway.RouteConfig, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" {
		return gateway.RouteConfig{}, nil
	}

	config := gateway.RouteConfig{Enabled: true}
	if err := json.Unmarshal([]byte(trimmed), &config); err != nil {
		return gateway.RouteConfig{}, err
	}
	return config, nil
}

// isRoutedGateway reports whether a Gateway enables external route exposure.
func isRoutedGateway(gw *pb.Gateway) bool {
	if gw.Route == nil {
		return false
	}
	routeConfig, err := parseGatewayRouteConfig(*gw.Route)
	if err != nil {
		// The provisioning path reports invalid configuration. Preserve the
		// exposure until that path can resolve the invalid value.
		return true
	}
	return routeConfig.Enabled
}

// gatewayNamespace returns the Kubernetes namespace a Gateway is deployed into.
// The namespace is assigned deterministically at creation (the API server's
// Gateway.BeforeCreate sets `openshell-<hex(ksuid)>`) and is carried on every
// event, so any Gateway that reaches a reconciler has one. It returns an error
// rather than synthesizing a name from gw.Name: a guessed namespace would
// diverge from the real `openshell-<hex(ksuid)>` scheme and, on the delete
// path, could hand a wrong (possibly live) namespace to the destructive
// DeleteManagedNamespace.
func gatewayNamespace(gw *pb.Gateway) (string, error) {
	ns := gw.GetNamespace()
	if ns == "" {
		return "", fmt.Errorf("gateway %s has no namespace", gw.GetMetadata().GetId())
	}
	return ns, nil
}

// gatewayListPageSize is the page size the reconcilers use when paging through
// the full gateway inventory over gRPC. It matches the API server's maximum
// page size so the common (small-fleet) case completes in a single request.
const gatewayListPageSize = 500

// listAllGateways pages through the gRPC gateway inventory and returns every
// gateway. The list endpoint is server-side paginated (default page size 20),
// so callers that must reason about the whole fleet (the namespace reaper and
// the health reconciler) cannot rely on a single unpaged request.
func listAllGateways(ctx context.Context, client pb.GatewayServiceClient) ([]*pb.Gateway, error) {
	var all []*pb.Gateway
	for page := int32(1); ; page++ {
		resp, err := client.ListGateways(ctx, &pb.ListGatewaysRequest{
			Page: page,
			Size: gatewayListPageSize,
		})
		if err != nil {
			return nil, err
		}
		items := resp.GetItems()
		all = append(all, items...)

		// Stop once we've collected the whole set (authoritative Total), or the
		// server returns a short/empty page. The latter two are defensive so a
		// misreported Total can never spin this loop forever.
		total := int(resp.GetMetadata().GetTotal())
		if len(items) == 0 || len(items) < gatewayListPageSize || (total > 0 && len(all) >= total) {
			return all, nil
		}
	}
}

// updateGatewayHealth sets the Gateway `phase` and `status` together in a single
// gRPC update so the console and CLI observe a consistent lifecycle state and
// health descriptor. It returns the stored Gateway on success so callers can
// use the API server timestamps. It returns nil if the update fails.
func (r *GatewayReconciler) updateGatewayHealth(ctx context.Context, gatewayID, phase, status string) *pb.Gateway {
	client := pb.NewGatewayServiceClient(r.grpcConn)
	response, err := client.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:     gatewayID,
		Phase:  &phase,
		Status: &status,
	})
	if err != nil {
		log.Printf("WARN failed to update gateway %s health to %s (%s): %v", gatewayID, phase, status, err)
		return nil
	}
	return response.GetGateway()
}

func (r *GatewayReconciler) updateGatewayPhase(ctx context.Context, gatewayID string, phase string) {
	client := pb.NewGatewayServiceClient(r.grpcConn)
	_, err := client.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:    gatewayID,
		Phase: &phase,
	})
	if err != nil {
		log.Printf("WARN failed to update gateway %s phase to %s: %v", gatewayID, phase, err)
	}
}

// updateGatewayStatus changes status without changing phase. Lightweight
// external-state repair uses it so a missing Keycloak client is visible without
// falsely claiming that the Kubernetes workload phase changed.
func (r *GatewayReconciler) updateGatewayStatus(ctx context.Context, gatewayID, gatewayStatus string) error {
	client := pb.NewGatewayServiceClient(r.grpcConn)
	if _, err := client.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:     gatewayID,
		Status: &gatewayStatus,
	}); err != nil {
		return fmt.Errorf("update gateway %s status: %w", gatewayID, err)
	}
	return nil
}

// consoleAddressFor returns the console_address a gateway should carry given
// whether its console Deployment is Ready: the console URL when Ready, empty
// otherwise. Publishing empty until the console pod can serve keeps the web UI's
// console button hidden, and retracts it if the pod later goes unready.
func consoleAddressFor(ready bool, url string) string {
	if ready {
		return url
	}
	return ""
}

// syncConsoleAddress publishes the gateway's console_address once its console is
// observed servable and clears it otherwise, so the web UI only offers the
// console button when the console can serve. The console Deployment and the
// selected exposure resource must both be Ready. It does not publish an address
// without a selected ingress mode. It leaves the address unchanged after a
// temporary observation error. It returns whether the console can serve.
func syncConsoleAddress(ctx context.Context, clientset kubernetes.Interface, dynamicClient dynamic.Interface, client pb.GatewayServiceClient, gatewayID string, gw *pb.Gateway, ingressMode string) bool {
	if gatewayID == "" || ingressMode == gateway.IngressModeNone || !isRoutedGateway(gw) {
		return false
	}
	namespace, err := gatewayNamespace(gw)
	if err != nil {
		log.Printf("WARN console address for %s: %v", gatewayID, err)
		return false
	}
	url, hasURL := gateway.ConsoleURL(namespace)
	ready := false
	if hasURL {
		ready, _, err = gateway.DeploymentReadiness(ctx, clientset, namespace, gateway.ConsoleDeploymentName)
		if err != nil {
			log.Printf("WARN console readiness for %s: %v", namespace, err)
			return false
		}
	}
	if ready {
		// The selected public exposure must be Ready before the reconciler
		// publishes the address.
		exposureReady, reason, exposureErr := gateway.ConsoleExposureReady(ctx, dynamicClient, namespace, ingressMode)
		if exposureErr != nil {
			log.Printf("WARN console exposure readiness for %s: %v", namespace, exposureErr)
			return false
		}
		if !exposureReady {
			log.Printf("INFO console for %s not servable yet: %s", namespace, reason)
			ready = false
		}
	}
	desired := consoleAddressFor(ready, url)
	if gw.GetConsoleAddress() == desired {
		return ready
	}
	if _, err := client.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:             gatewayID,
		ConsoleAddress: &desired,
	}); err != nil {
		log.Printf("WARN failed to set console address for %s to %q: %v", gatewayID, desired, err)
		return false
	}
	log.Printf("INFO console address for %s set to %q (consoleReady=%v)", gatewayID, desired, ready)
	return ready
}

// makeRouteAddressUpdater returns a RouteAddressUpdater callback that PATCHes
// the route_address field on the API-server Gateway via gRPC.
func (r *GatewayReconciler) makeRouteAddressUpdater(gatewayID string) gateway.RouteAddressUpdater {
	return func(ctx context.Context, routeAddress string) error {
		return r.updateRouteAddress(ctx, gatewayID, routeAddress)
	}
}

func (r *GatewayReconciler) updateRouteAddress(ctx context.Context, gatewayID string, routeAddress string) error {
	client := pb.NewGatewayServiceClient(r.grpcConn)
	_, err := client.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:           gatewayID,
		RouteAddress: &routeAddress,
	})
	if err != nil {
		return fmt.Errorf("update gateway %s route_address to %s: %w", gatewayID, routeAddress, err)
	}
	return nil
}

// makeConsoleAddressUpdater returns a ConsoleAddressUpdater callback that
// PATCHes the console_address field on the API-server Gateway via gRPC.
func (r *GatewayReconciler) makeConsoleAddressUpdater(gatewayID string) gateway.ConsoleAddressUpdater {
	return func(ctx context.Context, consoleAddress string) error {
		return r.updateConsoleAddress(ctx, gatewayID, consoleAddress)
	}
}

func (r *GatewayReconciler) updateConsoleAddress(ctx context.Context, gatewayID string, consoleAddress string) error {
	client := pb.NewGatewayServiceClient(r.grpcConn)
	_, err := client.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:             gatewayID,
		ConsoleAddress: &consoleAddress,
	})
	if err != nil {
		return fmt.Errorf("update gateway %s console_address to %s: %w", gatewayID, consoleAddress, err)
	}
	return nil
}

type managedDatabaseDeleteClient interface {
	DeleteManagedDatabase(ctx context.Context, in *pb.DeleteManagedDatabaseRequest, opts ...grpc.CallOption) (*pb.DeleteManagedDatabaseResponse, error)
}

func deleteGatewayDeploymentDatabase(ctx context.Context, client managedDatabaseDeleteClient, databaseID string) error {
	_, err := client.DeleteManagedDatabase(ctx, &pb.DeleteManagedDatabaseRequest{Id: databaseID})
	if status.Code(err) == codes.NotFound {
		return nil
	}
	if err != nil {
		return fmt.Errorf("delete ManagedDatabase %s: %w", databaseID, err)
	}
	return nil
}

type databaseConfig struct {
	Provider        string
	CNPG            gateway.CNPGConfig
	SourceNamespace string
}

func (r *GatewayReconciler) resolveDatabaseConfig(ctx context.Context, gw *pb.Gateway) (databaseConfig, error) {
	if gw.DatabaseId == "" {
		return databaseConfig{}, fmt.Errorf("gateway has no database_id; assign a ManagedDatabase to the gateway")
	}

	client := pb.NewManagedDatabaseServiceClient(r.grpcConn)
	resp, err := client.GetManagedDatabase(ctx, &pb.GetManagedDatabaseRequest{Id: gw.DatabaseId})
	if err != nil {
		return databaseConfig{}, fmt.Errorf("resolve ManagedDatabase %s: %w", gw.DatabaseId, err)
	}

	db := resp.ManagedDatabase
	if db == nil {
		return databaseConfig{}, fmt.Errorf("gateway configuration error: ManagedDatabase %s returned empty payload", gw.DatabaseId)
	}
	if db.Namespace == "" {
		return databaseConfig{}, fmt.Errorf("ManagedDatabase %s has no namespace assigned", gw.DatabaseId)
	}

	switch db.Provider {
	case "cnpg":
		return databaseConfig{
			Provider: "cnpg",
			CNPG: gateway.CNPGConfig{
				ClusterName:      "openshell-db",
				ClusterNamespace: db.Namespace,
			},
		}, nil
	case "deployment":
		return databaseConfig{
			Provider:        "deployment",
			SourceNamespace: db.Namespace,
		}, nil
	default:
		return databaseConfig{}, fmt.Errorf("ManagedDatabase %s has unsupported provider %q", gw.DatabaseId, db.Provider)
	}
}

func (r *GatewayReconciler) makeOIDCUpdater(gatewayID string) func(ctx context.Context, oidcJSON string) error {
	return func(ctx context.Context, oidcJSON string) error {
		client := pb.NewGatewayServiceClient(r.grpcConn)
		_, err := client.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
			Id:   gatewayID,
			Oidc: &oidcJSON,
		})
		if err != nil {
			return fmt.Errorf("update gateway %s oidc: %w", gatewayID, err)
		}
		return nil
	}
}

type StubGatewayReconciler struct{}

func NewStubGatewayReconciler() *StubGatewayReconciler {
	return &StubGatewayReconciler{}
}

func (r *StubGatewayReconciler) Handle(ctx context.Context, event watcher.Event[*pb.Gateway]) error {
	log.Printf("INFO [stub] reconciling Gateway %s (event=%d)", event.ResourceID, event.Type)
	return nil
}

// GatewayNetwork topology vocabulary. See
// specs/platform/gateway-network-reconciliation.spec.md.
const (
	networkTopologyMesh     = "mesh"
	networkTopologyHubSpoke = "hub-spoke"
)

// GatewayNetwork control-plane-owned status values. A network owns no Kubernetes
// resources in this scope, so status reflects configuration validity only, not
// provisioned connectivity.
const (
	networkStatusValid   = "Valid"
	networkStatusInvalid = "Invalid"
)

// GatewayNetworkReconciler reconciles GatewayNetwork resources. A network owns no
// Kubernetes resources in this scope, so reconciliation means: validate the
// network's topology vocabulary and topology/hub coherence, validate that a
// designated hub_gateway_id references an existing Gateway, and write a
// deterministic status back to the API server. Applying real gateway-to-gateway
// connectivity (mesh/tunnel provisioning) is future work owned by a sibling spec
// once product defines the network membership model and connectivity technology;
// this reconciler only records whether the declared configuration is well-formed.
type GatewayNetworkReconciler struct {
	mu     sync.Mutex
	active map[string]struct{}

	gateways pb.GatewayServiceClient
	networks pb.GatewayNetworkServiceClient
}

// NewGatewayNetworkReconciler builds the network reconciler. conn is the API
// server gRPC connection used to look up the designated hub gateway and to write
// network status back. conn may be nil (e.g. in unit tests), in which case the
// hub existence check and status write-back are skipped but the rest of
// validation still runs.
func NewGatewayNetworkReconciler(conn *grpc.ClientConn) *GatewayNetworkReconciler {
	r := &GatewayNetworkReconciler{active: make(map[string]struct{})}
	if conn != nil {
		r.gateways = pb.NewGatewayServiceClient(conn)
		r.networks = pb.NewGatewayNetworkServiceClient(conn)
	}
	return r
}

func (r *GatewayNetworkReconciler) Handle(ctx context.Context, event watcher.Event[*pb.GatewayNetwork]) error {
	r.mu.Lock()
	if _, ok := r.active[event.ResourceID]; ok {
		r.mu.Unlock()
		return nil
	}
	r.active[event.ResourceID] = struct{}{}
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.active, event.ResourceID)
		r.mu.Unlock()
	}()

	_, endSpan := cpotel.StartReconcileSpan(ctx, "GatewayNetwork", event.Type.String())
	var reconcileErr error
	defer func() { endSpan(reconcileErr) }()

	// A network owns no cluster resources, so a delete is a terminal, idempotent
	// no-op with respect to Kubernetes: gateways designated by the network are
	// left untouched.
	if event.Type == watcher.EventDeleted {
		log.Printf("INFO gateway network %s deleted; no cluster resources to remove", event.ResourceID)
		return nil
	}

	net := event.Resource
	if net == nil {
		log.Printf("WARN gateway network event %s has nil resource, skipping", event.ResourceID)
		return nil
	}

	// Validate the declared configuration. A transient dependency failure (e.g. a
	// transient hub lookup error) is returned so the failure is surfaced (logged
	// by the watch loop) rather than silently swallowed or settled to a misleading
	// Invalid. The network watch is inline log-only (no reconcile queue) and does
	// not replay state on reconnect, so a surfaced error re-converges only when the
	// network is next mutated, not automatically.
	desiredStatus, retryErr := r.validate(ctx, net)
	if retryErr != nil {
		reconcileErr = fmt.Errorf("validate gateway network %s: %w", event.ResourceID, retryErr)
		return reconcileErr
	}

	// Deterministic, idempotent status write-back: only update when the persisted
	// status differs from the reconciled outcome.
	if net.GetStatus() != desiredStatus {
		if err := r.updateStatus(ctx, event.ResourceID, desiredStatus); err != nil {
			reconcileErr = fmt.Errorf("update gateway network %s status: %w", event.ResourceID, err)
			return reconcileErr
		}
	}
	return nil
}

// validate applies the network's structural and referential coherence rules and
// returns the deterministic desired status (networkStatusValid, or
// "networkStatusInvalid: reason"). It returns a non-nil error only for a
// transient dependency failure that should be surfaced rather than swallowed; a
// definitive not-found for the hub gateway is a deterministic Invalid, not an
// error.
func (r *GatewayNetworkReconciler) validate(ctx context.Context, net *pb.GatewayNetwork) (string, error) {
	invalid := func(reason string) string {
		return fmt.Sprintf("%s: %s", networkStatusInvalid, reason)
	}

	topology := net.GetTopology()
	switch topology {
	case "":
		return invalid("topology is required"), nil
	case networkTopologyMesh, networkTopologyHubSpoke:
		// recognized
	default:
		return invalid(fmt.Sprintf("unrecognized topology %q", topology)), nil
	}

	hubID := net.GetHubGatewayId()
	if topology == networkTopologyHubSpoke && hubID == "" {
		return invalid("hub-spoke network requires a hub_gateway_id"), nil
	}

	if hubID != "" {
		// A configured hub must reference an existing Gateway. Skip the lookup when
		// no gateway client is configured (started without an API-server gRPC
		// connection, e.g. in unit tests).
		if r.gateways == nil {
			return networkStatusValid, nil
		}
		_, err := r.gateways.GetGateway(ctx, &pb.GetGatewayRequest{Id: hubID})
		if err != nil {
			if status.Code(err) == codes.NotFound {
				return invalid(fmt.Sprintf("hub gateway %q does not exist", hubID)), nil
			}
			// Transient failure: surface as an error rather than settle to a
			// misleading Invalid.
			return "", err
		}
	}

	return networkStatusValid, nil
}

// updateStatus writes the network's reconciled status back to the API server. It
// is a no-op when the network client is not configured.
func (r *GatewayNetworkReconciler) updateStatus(ctx context.Context, id, desired string) error {
	if r.networks == nil {
		return nil
	}
	_, err := r.networks.UpdateGatewayNetwork(ctx, &pb.UpdateGatewayNetworkRequest{
		Id:     id,
		Status: &desired,
	})
	return err
}
