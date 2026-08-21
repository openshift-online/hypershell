package reconciler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/exposure"
	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
	"github.com/openshift-online/hypershell/components/control-plane/internal/keycloak"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

type FleetReconciler struct {
	mu     sync.Mutex
	active map[string]struct{}
}

func NewFleetReconciler() *FleetReconciler {
	return &FleetReconciler{active: make(map[string]struct{})}
}

func (r *FleetReconciler) Handle(ctx context.Context, event watcher.Event[*pb.Fleet]) error {
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

	log.Printf("INFO reconciling Fleet %s (event=%d)", event.ResourceID, event.Type)
	return nil
}

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

	log.Printf("INFO reconciling ManagedCluster %s (event=%d)", event.ResourceID, event.Type)
	return nil
}

type ManagedDatabaseReconciler struct {
	mu            sync.Mutex
	active        map[string]struct{}
	dynamicClient dynamic.Interface
	clientset     *kubernetes.Clientset
	grpcConn      *grpc.ClientConn
	hasCNPG       bool
}

func NewManagedDatabaseReconciler(
	dynamicClient dynamic.Interface,
	clientset *kubernetes.Clientset,
	grpcConn *grpc.ClientConn,
) *ManagedDatabaseReconciler {
	hasCNPG := gateway.DetectCNPG(clientset)
	return &ManagedDatabaseReconciler{
		active:        make(map[string]struct{}),
		dynamicClient: dynamicClient,
		clientset:     clientset,
		grpcConn:      grpcConn,
		hasCNPG:       hasCNPG,
	}
}

func (r *ManagedDatabaseReconciler) Handle(ctx context.Context, event watcher.Event[*pb.ManagedDatabase]) error {
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

	db := event.Resource
	if db == nil {
		log.Printf("WARN ManagedDatabase event %s has nil resource, skipping", event.ResourceID)
		return nil
	}

	if db.Provider != "cnpg" {
		log.Printf("WARN ManagedDatabase %s has unsupported provider %q, skipping", event.ResourceID, db.Provider)
		return nil
	}

	if event.Type == watcher.EventDeleted {
		log.Printf("INFO ManagedDatabase %s deleted, cleaning up CNPG cluster in namespace %s", event.ResourceID, db.Namespace)
		r.deleteCNPGCluster(ctx, db.Namespace)
		return nil
	}

	log.Printf("INFO reconciling ManagedDatabase %s name=%s namespace=%s (event=%d)",
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

func (r *ManagedDatabaseReconciler) reconcileCNPGCluster(ctx context.Context, db *pb.ManagedDatabase) error {
	namespace := db.Namespace

	if !gateway.NamespaceExists(ctx, r.clientset, namespace) {
		if err := gateway.CreateManagedNamespace(ctx, r.clientset, namespace); err != nil {
			return fmt.Errorf("create namespace %s: %w", namespace, err)
		}
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

func (r *ManagedDatabaseReconciler) deleteCNPGCluster(ctx context.Context, namespace string) {
	clusterName := managedDatabaseCNPGClusterName()

	if err := r.dynamicClient.Resource(cnpgClusterGVR()).Namespace(namespace).Delete(ctx, clusterName, metav1.DeleteOptions{}); err != nil {
		if !k8serrors.IsNotFound(err) {
			log.Printf("WARN failed to delete CNPG Cluster %s/%s: %v", namespace, clusterName, err)
		}
	} else {
		log.Printf("INFO deleted CNPG Cluster %s/%s", namespace, clusterName)
	}

	if err := r.clientset.CoreV1().Namespaces().Delete(ctx, namespace, metav1.DeleteOptions{}); err != nil {
		if !k8serrors.IsNotFound(err) {
			log.Printf("WARN failed to delete namespace %s: %v", namespace, err)
		}
	} else {
		log.Printf("INFO deleted namespace %s", namespace)
	}
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

	log.Printf("INFO gateway reconciler initialized: manifests=%d openshift=%v certmanager=%v gatewayapi=%v cnpg=%v keycloak=%v netpol=%v",
		len(manifests), isOpenShift, hasCertManager, hasGatewayAPI, hasCNPG, kcClient != nil, !skipNetworkPolicies)

	return &GatewayReconciler{
		active:                make(map[string]struct{}),
		dynamicClient:         dynamicClient,
		clientset:             clientset,
		grpcConn:              grpcConn,
		manifests:             manifests,
		isOpenShift:           isOpenShift,
		hasCertManager:        hasCertManager,
		hasGatewayAPI:         hasGatewayAPI,
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

	if event.Type == watcher.EventDeleted {
		namespace, err := gatewayNamespace(gw)
		if err != nil {
			// Without a recorded namespace there is nothing deterministic to
			// clean up, and guessing one could delete the wrong (possibly live)
			// namespace. Skip; the NamespaceGCReconciler is the backstop for any
			// orphaned managed namespace.
			log.Printf("WARN gateway %s deleted but %v; skipping namespace cleanup", event.ResourceID, err)
			return nil
		}

		deleteCNPGConfig, cnpgErr := r.resolveCNPGConfig(ctx, gw)
		if cnpgErr != nil {
			log.Printf("WARN gateway %s deleted but could not resolve CNPG config: %v; CNPG resources may require manual cleanup", event.ResourceID, cnpgErr)
		}
		log.Printf("INFO gateway %s deleted, cleaning up resources in namespace %s", event.ResourceID, namespace)
		opts := gateway.ReconcileOpts{
			IsOpenShift:           r.isOpenShift,
			HasCertManager:        r.hasCertManager,
			HasGatewayAPI:         r.hasGatewayAPI,
			SkipNetworkPolicies:   r.skipNetworkPolicies,
			HasCNPG:               r.hasCNPG,
			CNPG:                  deleteCNPGConfig,
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
			return fmt.Errorf("delete gateway resources in %s: %w", namespace, err)
		}
		log.Printf("INFO gateway %s resources cleaned up from namespace %s", event.ResourceID, namespace)

		// Delete the gateway's namespace itself. This is best-effort and
		// idempotent: DeleteManagedNamespace only removes namespaces this control
		// plane manages and treats an already-absent namespace as success. Any
		// namespace missed here (e.g. a delete event lost during a restart) is
		// swept later by the NamespaceGCReconciler.
		deleted, err := gateway.DeleteManagedNamespace(ctx, r.clientset, namespace)
		if err != nil {
			return fmt.Errorf("delete gateway namespace %s: %w", namespace, err)
		}
		if !deleted {
			// The namespace was left in place: it is already gone, still
			// terminating, or - the case that matters - a pre-existing namespace
			// this control plane does not manage. In that last case deleting the
			// namespace never cascades this gateway's in-namespace objects, and the
			// NamespaceGCReconciler skips unmanaged namespaces too, so reclaim the
			// resources this gateway labeled without touching the shared namespace or
			// any co-tenant workloads.
			gateway.DeleteLabeledNamespaceResources(ctx, r.dynamicClient, namespace, opts)
		}
		return nil
	}

	log.Printf("INFO reconciling Gateway %s name=%s namespace=%s (event=%d)",
		event.ResourceID, gw.Name, gw.Namespace, event.Type)

	// The phase gate prevents redundant re-provisioning (re-applying manifests)
	// of a Gateway that has already been acted upon. Running, Provisioning, and
	// Degraded gateways are owned by the continuous health reconciler, which
	// keeps their phase synchronized with workload health via a separate path
	// that this gate does not suppress. See openshell-gateway-health.spec.md.
	if gw.Phase != nil && (*gw.Phase == "Running" || *gw.Phase == "Provisioning" || *gw.Phase == "Degraded") {
		log.Printf("DEBUG gateway %s phase=%s, skipping reconciliation", event.ResourceID, *gw.Phase)
		return nil
	}

	reconcileDatabase := gw.DatabaseId != ""
	var cnpgConfig gateway.CNPGConfig
	if reconcileDatabase {
		var resolveErr error
		cnpgConfig, resolveErr = r.resolveCNPGConfig(ctx, gw)
		if resolveErr != nil {
			return fmt.Errorf("resolve CNPG config for gateway %s: %w", gw.Name, resolveErr)
		}
	} else {
		log.Printf("INFO gateway %s has no database_id; skipping database reconciliation (existing database resources left untouched)", event.ResourceID)
	}

	namespace, err := gatewayNamespace(gw)
	if err != nil {
		return fmt.Errorf("reconcile gateway %s: %w", gw.Name, err)
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
			return fmt.Errorf("invalid oidc config for gateway %s: %w", gw.Name, err)
		}
		gwConfig.OIDC = oidcConfig
	}

	if gw.Route != nil && *gw.Route != "" {
		var routeConfig gateway.RouteConfig
		if err := json.Unmarshal([]byte(*gw.Route), &routeConfig); err != nil {
			return fmt.Errorf("invalid route config for gateway %s: %w", gw.Name, err)
		}
		gwConfig.Route = routeConfig
	}

	if gw.CredentialDriver != nil && *gw.CredentialDriver != "" {
		var credDriverConfig gateway.CredentialDriverConfig
		decoder := json.NewDecoder(bytes.NewReader([]byte(*gw.CredentialDriver)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&credDriverConfig); err != nil {
			return fmt.Errorf("invalid credential driver config for gateway %s: %w", gw.Name, err)
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
		ReconcileDatabase:     reconcileDatabase,
		CNPG:                  cnpgConfig,
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

	r.updateGatewayPhase(ctx, event.ResourceID, "Provisioning")

	if err := gateway.ReconcileGateway(ctx, r.dynamicClient, r.clientset, nsConfig, r.manifests, opts); err != nil {
		r.updateGatewayPhase(ctx, event.ResourceID, "Failed")
		return fmt.Errorf("reconcile gateway %s: %w", gw.Name, err)
	}

	// Manifests are applied, but the gateway is not Running until its workload is
	// observed Ready. Wait within the provisioning readiness window; if the
	// Deployment never becomes ready, set Degraded and record why.
	ready, reason := gateway.WaitForGatewayReady(ctx, r.clientset, namespace, 2*time.Minute)
	if !ready {
		r.updateGatewayHealth(ctx, event.ResourceID, "Degraded", reason)
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
	if r.exposure != nil && isRoutedGateway(gw) {
		if r.waitForRouteReady(ctx, namespace) {
			r.updateGatewayHealth(ctx, event.ResourceID, "Running", "Healthy")
			log.Printf("INFO gateway %s provisioned and route ready in namespace %s", gw.Name, namespace)
		} else {
			r.updateGatewayHealth(ctx, event.ResourceID, "Provisioning", "Deployment ready; awaiting route readiness")
			log.Printf("INFO gateway %s deployment ready in namespace %s; awaiting route readiness", gw.Name, namespace)
		}
		// The console pod starts after the gateway is routed and typically becomes
		// Ready seconds to a minute later. Poll its readiness on a tight cadence in
		// the background and publish console_address as soon as it can serve, so the
		// web UI's console button enables promptly instead of waiting for the next
		// 30s health-reconciler tick. It runs in the background so a slow console
		// image pull never blocks the (serial) gateway watch loop; the health
		// reconciler remains the backstop that publishes and retracts the address.
		go r.publishConsoleAddressWhenReady(ctx, event.ResourceID, gw)
	} else {
		r.updateGatewayHealth(ctx, event.ResourceID, "Running", "Healthy")
		log.Printf("INFO gateway %s provisioned and ready in namespace %s", gw.Name, namespace)
	}
	return nil
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
		return syncConsoleAddress(ctx, r.clientset, r.dynamicClient, client, gatewayID, current, r.exposure != nil)
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

// isRoutedGateway reports whether a Gateway declares external route exposure
// (a non-empty `route` configuration), and therefore requires its external
// exposure to be observed Ready before it can be reported Running.
func isRoutedGateway(gw *pb.Gateway) bool {
	if gw.Route == nil {
		return false
	}
	route := strings.TrimSpace(*gw.Route)
	return route != "" && route != "null"
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
// health descriptor.
func (r *GatewayReconciler) updateGatewayHealth(ctx context.Context, gatewayID, phase, status string) {
	client := pb.NewGatewayServiceClient(r.grpcConn)
	_, err := client.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:     gatewayID,
		Phase:  &phase,
		Status: &status,
	})
	if err != nil {
		log.Printf("WARN failed to update gateway %s health to %s (%s): %v", gatewayID, phase, status, err)
	}
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
// console button when the console can actually serve. "Servable" requires both
// the console Deployment to be Ready AND the console HTTPRoute to be accepted
// (Accepted + ResolvedRefs on the shared Gateway listener) -- a Ready Deployment
// alone does not prove the public route works, and publishing the address then
// would enable a dead link. It is a no-op for gateways without a console (no
// exposure port, or not routed) and when the base domain is unconfigured, and it
// leaves the address untouched on a transient readiness-observation error rather
// than flapping the button. It returns whether the console is currently servable,
// so a caller polling during provisioning can stop once the address is published.
func syncConsoleAddress(ctx context.Context, clientset *kubernetes.Clientset, dynamicClient dynamic.Interface, client pb.GatewayServiceClient, gatewayID string, gw *pb.Gateway, hasExposure bool) bool {
	if gatewayID == "" || !hasExposure || !isRoutedGateway(gw) {
		return false
	}
	namespace, err := gatewayNamespace(gw)
	if err != nil {
		log.Printf("WARN console address for %s: %v", gatewayID, err)
		return false
	}
	url, ok := gateway.ConsoleURL(namespace)
	if !ok {
		return false
	}
	ready, _, err := gateway.DeploymentReadiness(ctx, clientset, namespace, gateway.ConsoleDeploymentName)
	if err != nil {
		log.Printf("WARN console readiness for %s: %v", namespace, err)
		return false
	}
	if ready {
		// The Deployment is Ready; require the public route to be accepted too
		// before publishing the address, logging the listener rejection reason
		// otherwise so a misconfigured HTTP listener is diagnosable.
		routeReady, reason, routeErr := gateway.ConsoleRouteReady(ctx, dynamicClient, namespace)
		if routeErr != nil {
			log.Printf("WARN console route readiness for %s: %v", namespace, routeErr)
			return false
		}
		if !routeReady {
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

func (r *GatewayReconciler) resolveCNPGConfig(ctx context.Context, gw *pb.Gateway) (gateway.CNPGConfig, error) {
	if gw.DatabaseId == "" {
		return gateway.CNPGConfig{}, fmt.Errorf("gateway has no database_id; assign a ManagedDatabase to the fleet")
	}

	client := pb.NewManagedDatabaseServiceClient(r.grpcConn)
	resp, err := client.GetManagedDatabase(ctx, &pb.GetManagedDatabaseRequest{Id: gw.DatabaseId})
	if err != nil {
		return gateway.CNPGConfig{}, fmt.Errorf("resolve ManagedDatabase %s: %w", gw.DatabaseId, err)
	}

	db := resp.ManagedDatabase
	if db == nil {
		return gateway.CNPGConfig{}, fmt.Errorf("gateway configuration error: ManagedDatabase %s returned empty payload", gw.DatabaseId)
	}
	if db.Provider != "cnpg" {
		return gateway.CNPGConfig{}, fmt.Errorf("gateway database_id references a non-CNPG managed database; only provider=cnpg is supported")
	}
	if db.Namespace == "" {
		return gateway.CNPGConfig{}, fmt.Errorf("ManagedDatabase %s has no namespace assigned", gw.DatabaseId)
	}

	return gateway.CNPGConfig{
		ClusterName:      "openshell-db",
		ClusterNamespace: db.Namespace,
	}, nil
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

type GatewayNetworkReconciler struct {
	mu     sync.Mutex
	active map[string]struct{}
}

func NewGatewayNetworkReconciler() *GatewayNetworkReconciler {
	return &GatewayNetworkReconciler{active: make(map[string]struct{})}
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

	log.Printf("INFO reconciling GatewayNetwork %s (event=%d)", event.ResourceID, event.Type)
	return nil
}
