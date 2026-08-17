package gateway

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lib/pq"
	"github.com/openshift-online/hypershell/components/control-plane/internal/exposure"
	"github.com/openshift-online/hypershell/components/control-plane/internal/keycloak"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/watch"
)

// networkPoliciesDisabledLogOnce keeps the "network policies disabled" notice to
// a single line per process. When GATEWAY_SKIP_NETWORK_POLICIES=true (the
// default in the kind overlay) the skip branches run on every reconcile, so
// logging per-resource produced steady per-reconcile noise under a misleading
// DEBUG label. logNetworkPoliciesDisabled emits the notice once instead.
var networkPoliciesDisabledLogOnce sync.Once

func logNetworkPoliciesDisabled() {
	networkPoliciesDisabledLogOnce.Do(func() {
		log.Printf("network policies disabled (GATEWAY_SKIP_NETWORK_POLICIES=true); skipping gateway NetworkPolicy resources")
	})
}

func ReconcileGateway(
	ctx context.Context,
	dynamicClient dynamic.Interface,
	clientset *kubernetes.Clientset,
	nsConfig NamespaceConfig,
	manifests map[string][]*unstructured.Unstructured,
	opts ReconcileOpts,
) error {
	images := opts.Images
	if images == nil {
		images = StaticImageDefaults{}
	}

	if !namespaceExists(ctx, clientset, nsConfig.Name) {
		if err := createNamespace(ctx, clientset, nsConfig.Name); err != nil {
			return fmt.Errorf("create namespace %s: %w", nsConfig.Name, err)
		}
	}

	if err := ValidateGatewayConfig(nsConfig.Gateway); err != nil {
		return fmt.Errorf("invalid gateway configuration: %w", err)
	}

	dbImage := nsConfig.Gateway.Database.Image
	if dbImage == "" {
		dbImage = images.DefaultDatabaseImage()
	}

	if err := reconcileDatabaseCredentials(ctx, clientset, nsConfig.Name, dbImage); err != nil {
		return fmt.Errorf("reconcile database credentials in %s: %w", nsConfig.Name, err)
	}

	if opts.RotateDBCredentials != "" {
		if err := rotateDatabaseCredentials(ctx, clientset, nsConfig.Name, dbImage, opts.RotateDBCredentials); err != nil {
			return fmt.Errorf("rotate database credentials in %s: %w", nsConfig.Name, err)
		}
	}

	if nsConfig.Gateway.CredentialDriver == nil {
		if err := reconcileCredentialKEK(ctx, clientset, nsConfig.Name); err != nil {
			return fmt.Errorf("reconcile credential KEK in %s: %w", nsConfig.Name, err)
		}
		deleteCredentialSecretsRBAC(ctx, dynamicClient, nsConfig.Name)
	} else {
		if err := reconcileCredentialDriverResources(ctx, dynamicClient, clientset, nsConfig); err != nil {
			return fmt.Errorf("reconcile credential driver resources in %s: %w", nsConfig.Name, err)
		}
	}

	if opts.HasCertManager {
		if err := reconcileCertManagerResources(ctx, dynamicClient, nsConfig); err != nil {
			return fmt.Errorf("reconcile cert-manager resources in %s: %w", nsConfig.Name, err)
		}
	} else {
		return fmt.Errorf("cert-manager is required but not available on the cluster: gateway deployment blocked for namespace %s", nsConfig.Name)
	}

	if opts.Keycloak != nil {
		if err := reconcileKeycloakClient(ctx, opts, &nsConfig); err != nil {
			return fmt.Errorf("reconcile keycloak client in %s: %w", nsConfig.Name, err)
		}
	}

	hasTrustedCA := reconcileTrustedCABundle(ctx, clientset, opts.ControlPlaneNamespace, nsConfig.Name)

	if err := deployGateway(ctx, dynamicClient, clientset, nsConfig, manifests, images, opts, hasTrustedCA); err != nil {
		return fmt.Errorf("deploy gateway in %s: %w", nsConfig.Name, err)
	}

	if opts.IsOpenShift {
		if err := reconcileOpenShiftSCC(ctx, dynamicClient, nsConfig.Name); err != nil {
			log.Printf("WARN failed to reconcile OpenShift SCC binding in %s: %v", nsConfig.Name, err)
		}
	}

	if opts.HasGatewayAPI {
		if nsConfig.Gateway.Route.Enabled {
			if err := reconcileGatewayAPIResources(ctx, dynamicClient, clientset, nsConfig, opts); err != nil {
				log.Printf("WARN failed to reconcile Gateway API resources in %s: %v", nsConfig.Name, err)
			}
		} else {
			if err := deleteGatewayAPIResources(ctx, dynamicClient, clientset, nsConfig.Name, opts); err != nil {
				log.Printf("WARN failed to remove Gateway API resources in %s: %v", nsConfig.Name, err)
			}
		}
	}

	log.Printf("INFO gateway reconciled in namespace %s", nsConfig.Name)
	return nil
}

func DeleteGatewayResources(
	ctx context.Context,
	dynamicClient dynamic.Interface,
	clientset *kubernetes.Clientset,
	namespace string,
	opts ReconcileOpts,
	credentialNamespaces ...string,
) error {
	labelSelector := "hypershell.redhat.io/managed=true"

	namespacedResources := []schema.GroupVersionResource{
		{Group: "apps", Version: "v1", Resource: "deployments"},
		{Version: "v1", Resource: "services"},
		{Version: "v1", Resource: "configmaps"},
		{Version: "v1", Resource: "serviceaccounts"},
		{Version: "v1", Resource: "secrets"},
		{Version: "v1", Resource: "persistentvolumeclaims"},
		{Group: "batch", Version: "v1", Resource: "jobs"},
		{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"},
		{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles"},
		{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings"},
	}

	if opts.HasCertManager {
		namespacedResources = append(namespacedResources,
			schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "issuers"},
			schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"},
		)
	}

	if opts.HasGatewayAPI {
		namespacedResources = append(namespacedResources,
			schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "grpcroutes"},
			schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "backendtlspolicies"},
		)
	}

	for _, gvr := range namespacedResources {
		list, err := dynamicClient.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: labelSelector,
		})
		if err != nil {
			if k8serrors.IsNotFound(err) {
				continue
			}
			log.Printf("WARN failed to list %s in %s: %v", gvr.Resource, namespace, err)
			continue
		}
		for _, item := range list.Items {
			if err := dynamicClient.Resource(gvr).Namespace(namespace).Delete(ctx, item.GetName(), metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
				log.Printf("WARN failed to delete %s %s in %s: %v", gvr.Resource, item.GetName(), namespace, err)
			} else {
				log.Printf("INFO deleted %s %s from %s", gvr.Resource, item.GetName(), namespace)
			}
		}
	}

	crbGVR := schema.GroupVersionResource{
		Group:    "rbac.authorization.k8s.io",
		Version:  "v1",
		Resource: "clusterrolebindings",
	}
	crbName := fmt.Sprintf("openshell-gateway-node-reader-%s", namespace)
	if err := dynamicClient.Resource(crbGVR).Delete(ctx, crbName, metav1.DeleteOptions{}); err != nil {
		if !k8serrors.IsNotFound(err) {
			log.Printf("WARN failed to delete ClusterRoleBinding %s: %v", crbName, err)
		}
	} else {
		log.Printf("INFO deleted ClusterRoleBinding %s", crbName)
	}

	if opts.KeycloakClient != nil && opts.GatewayName != "" && opts.GatewayID != "" {
		kcClientID := fmt.Sprintf("%s-%s", opts.GatewayName, opts.GatewayID)
		if err := opts.KeycloakClient.DeleteGatewayClient(ctx, kcClientID); err != nil {
			log.Printf("WARN failed to delete keycloak client %s (orphaned): %v", kcClientID, err)
		} else {
			log.Printf("INFO deleted keycloak client %s", kcClientID)
		}
	}

	for _, credNS := range credentialNamespaces {
		if credNS != "" && credNS != namespace {
			deleteCredentialSecretsRBAC(ctx, dynamicClient, credNS)
			log.Printf("INFO cleaned up credential RBAC from namespace %s", credNS)
		}
	}

	log.Printf("INFO gateway resources cleaned up from namespace %s", namespace)
	return nil
}

func deleteGatewayAPIResources(ctx context.Context, dynamicClient dynamic.Interface, clientset *kubernetes.Clientset, namespace string, opts ReconcileOpts) error {
	grpcRouteGVR := schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1",
		Resource: "grpcroutes",
	}
	if err := dynamicClient.Resource(grpcRouteGVR).Namespace(namespace).Delete(ctx, "openshell-gateway", metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
		log.Printf("WARN failed to delete GRPCRoute: %v", err)
	}

	btlsGVR := schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1",
		Resource: "backendtlspolicies",
	}
	if err := dynamicClient.Resource(btlsGVR).Namespace(namespace).Delete(ctx, "openshell-gateway", metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
		log.Printf("WARN failed to delete BackendTLSPolicy: %v", err)
	}

	if err := clientset.CoreV1().ConfigMaps(namespace).Delete(ctx, "openshell-backend-ca", metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
		log.Printf("WARN failed to delete backend CA ConfigMap: %v", err)
	}

	netpolGVR := schema.GroupVersionResource{
		Group:    "networking.k8s.io",
		Version:  "v1",
		Resource: "networkpolicies",
	}
	if err := dynamicClient.Resource(netpolGVR).Namespace(namespace).Delete(ctx, "openshell-gateway-allow-router", metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
		log.Printf("WARN failed to delete router NetworkPolicy: %v", err)
	}

	if opts.UpdateRouteAddress != nil {
		if err := opts.UpdateRouteAddress(ctx, ""); err != nil {
			log.Printf("WARN failed to clear routeAddress for gateway in %s: %v", namespace, err)
		} else {
			log.Printf("INFO cleared routeAddress for gateway in %s", namespace)
		}
	}

	log.Printf("INFO Gateway API resources removed from namespace %s", namespace)
	return nil
}

func namespaceExists(ctx context.Context, clientset *kubernetes.Clientset, namespace string) bool {
	_, err := clientset.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	return err == nil
}

func createNamespace(ctx context.Context, clientset *kubernetes.Clientset, namespace string) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "hypershell-control-plane",
				"hypershell.redhat.io/managed": "true",
			},
		},
	}

	_, err := clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("create namespace: %w", err)
	}
	log.Printf("INFO created namespace %s", namespace)
	return nil
}

func deployGateway(
	ctx context.Context,
	dynamicClient dynamic.Interface,
	clientset *kubernetes.Clientset,
	nsConfig NamespaceConfig,
	manifests map[string][]*unstructured.Unstructured,
	images ImageDefaults,
	opts ReconcileOpts,
	hasTrustedCA bool,
) error {
	order := []string{
		"rbac.yaml",
		"serviceaccount.yaml",
		"configmap.yaml",
		"certgen-job.yaml",
		"database.yaml",
		"service.yaml",
		"deployment.yaml",
		"networkpolicy.yaml",
	}

	for _, filename := range order {
		resources, ok := manifests[filename]
		if !ok {
			log.Printf("WARN manifest file %s not found, skipping", filename)
			continue
		}

		for _, manifest := range resources {
			// Dev clusters skip the per-tenant gateway NetworkPolicies (they
			// would blackhole ingress from an out-of-cluster proxy whose source
			// IP no selector can match). This drops the netpol docs in both
			// networkpolicy.yaml and database.yaml while keeping the rest.
			if opts.SkipNetworkPolicies && manifest.GetKind() == "NetworkPolicy" {
				logNetworkPoliciesDisabled()
				continue
			}

			// Apply database overrides on a copy first so that
			// DB_IMAGE_PLACEHOLDER is resolved before the generic
			// IMAGE_PLACEHOLDER replacement runs (substring overlap).
			raw := manifest.DeepCopy()
			if err := ApplyDatabaseOverrides(raw, nsConfig.Gateway.Database, images); err != nil {
				return fmt.Errorf("apply database overrides for %s: %w", filename, err)
			}

			obj, err := ApplyManifestToNamespace(raw, nsConfig.Name, nsConfig.Gateway, images)
			if err != nil {
				return fmt.Errorf("apply substitutions for %s: %w", filename, err)
			}

			if err := ApplyConfigOverrides(obj, nsConfig.Gateway, nsConfig.Name); err != nil {
				return fmt.Errorf("apply config overrides for %s: %w", filename, err)
			}

			if obj.GetKind() == "Deployment" {
				applyConfigHashAnnotation(ctx, clientset, obj, nsConfig.Name)
			}

			if hasTrustedCA && obj.GetKind() == "Deployment" {
				applyTrustedCAOverrides(obj)
			}

			if opts.IsOpenShift && obj.GetKind() == "Deployment" {
				applyOpenShiftOverrides(obj)
			}

			if err := reconcileResource(ctx, dynamicClient, obj); err != nil {
				return fmt.Errorf("reconcile resource from %s: %w", filename, err)
			}

			log.Printf("DEBUG reconciled %s %s in %s", obj.GetKind(), obj.GetName(), nsConfig.Name)
		}

		if filename == "database.yaml" {
			if err := waitForDeploymentReady(ctx, clientset, nsConfig.Name, "openshell-gateway-db", 2*time.Minute); err != nil {
				return fmt.Errorf("wait for database in %s: %w", nsConfig.Name, err)
			}
		}
	}

	return nil
}

func waitForSecret(ctx context.Context, clientset *kubernetes.Clientset, namespace, name string, timeout time.Duration) error {
	watchCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	fieldSelector := fields.OneTermEqualSelector("metadata.name", name).String()

	for {
		list, err := clientset.CoreV1().Secrets(namespace).List(watchCtx, metav1.ListOptions{
			FieldSelector: fieldSelector,
		})
		if err != nil {
			if watchCtx.Err() != nil {
				return fmt.Errorf("timed out waiting for secret %s/%s: %w", namespace, name, watchCtx.Err())
			}
			return fmt.Errorf("list secret %s/%s: %w", namespace, name, err)
		}
		if len(list.Items) > 0 {
			return nil
		}

		watcher, err := clientset.CoreV1().Secrets(namespace).Watch(watchCtx, metav1.ListOptions{
			FieldSelector:   fieldSelector,
			ResourceVersion: list.ResourceVersion,
		})
		if err != nil {
			if watchCtx.Err() != nil {
				return fmt.Errorf("timed out waiting for secret %s/%s: %w", namespace, name, watchCtx.Err())
			}
			return fmt.Errorf("watch secret %s/%s: %w", namespace, name, err)
		}

		appeared := false
		for event := range watcher.ResultChan() {
			if event.Type == watch.Added || event.Type == watch.Modified {
				appeared = true
				break
			}
		}
		watcher.Stop()

		if appeared {
			log.Printf("INFO secret %s/%s is available", namespace, name)
			return nil
		}

		if watchCtx.Err() != nil {
			return fmt.Errorf("timed out waiting for secret %s/%s: %w", namespace, name, watchCtx.Err())
		}
		log.Printf("INFO watch for secret %s/%s closed early; re-establishing", namespace, name)
	}
}

// GatewayDeploymentName is the name of the primary gateway workload Deployment
// whose readiness gates the Gateway `Running` phase.
const GatewayDeploymentName = "openshell-gateway"

// DeploymentReadiness performs a single, non-blocking check of a Deployment's
// readiness. It returns ready=true when ready replicas meet or exceed desired
// replicas. When the Deployment is not ready, reason carries a short
// human-readable descriptor (e.g. "1/2 replicas ready" or "deployment not
// found") suitable for the Gateway `status` field.
func DeploymentReadiness(ctx context.Context, clientset *kubernetes.Clientset, namespace, name string) (ready bool, reason string, err error) {
	deploy, err := clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return false, "deployment not found", nil
		}
		return false, "", fmt.Errorf("get deployment %s/%s: %w", namespace, name, err)
	}

	desired := int32(1)
	if deploy.Spec.Replicas != nil {
		desired = *deploy.Spec.Replicas
	}
	if deploy.Status.ReadyReplicas >= desired {
		return true, "", nil
	}
	return false, fmt.Sprintf("%d/%d replicas ready", deploy.Status.ReadyReplicas, desired), nil
}

// WaitForGatewayReady blocks until the openshell-gateway Deployment reaches
// readiness or the timeout elapses. It returns ready=true on readiness, or
// ready=false with the last observed reason when the provisioning readiness
// window expires without the workload becoming ready.
func WaitForGatewayReady(ctx context.Context, clientset *kubernetes.Clientset, namespace string, timeout time.Duration) (bool, string) {
	deadline := time.After(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	lastReason := "not ready"
	for {
		select {
		case <-ctx.Done():
			return false, ctx.Err().Error()
		case <-deadline:
			return false, lastReason
		case <-ticker.C:
			ready, reason, err := DeploymentReadiness(ctx, clientset, namespace, GatewayDeploymentName)
			if err != nil {
				lastReason = err.Error()
				continue
			}
			if ready {
				return true, ""
			}
			if reason != "" {
				lastReason = reason
			}
		}
	}
}

func waitForDeploymentReady(ctx context.Context, clientset *kubernetes.Clientset, namespace, name string, timeout time.Duration) error {
	deadline := time.After(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timed out waiting for deployment %s/%s to become ready", namespace, name)
		case <-ticker.C:
			deploy, err := clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				if !k8serrors.IsNotFound(err) {
					log.Printf("WARN error checking deployment %s/%s readiness: %v", namespace, name, err)
				}
				continue
			}
			if deploy.Spec.Replicas != nil && deploy.Status.ReadyReplicas >= *deploy.Spec.Replicas {
				log.Printf("INFO deployment %s/%s is ready", namespace, name)
				return nil
			}
		}
	}
}

func reconcileResource(ctx context.Context, dynamicClient dynamic.Interface, obj *unstructured.Unstructured) error {
	gvk := obj.GroupVersionKind()
	gvr := schema.GroupVersionResource{
		Group:    gvk.Group,
		Version:  gvk.Version,
		Resource: kindToResource(gvk.Kind),
	}

	namespace := obj.GetNamespace()
	name := obj.GetName()

	var resourceClient dynamic.ResourceInterface
	if namespace != "" {
		resourceClient = dynamicClient.Resource(gvr).Namespace(namespace)
	} else {
		resourceClient = dynamicClient.Resource(gvr)
	}

	existing, err := resourceClient.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			_, err = resourceClient.Create(ctx, obj, metav1.CreateOptions{})
			if err != nil {
				return fmt.Errorf("create %s %s: %w", gvk.Kind, name, err)
			}
			log.Printf("INFO created %s %s in %s", gvk.Kind, name, namespace)
			return nil
		}
		return fmt.Errorf("get %s %s: %w", gvk.Kind, name, err)
	}

	if gvk.Kind == "Job" {
		log.Printf("DEBUG job %s already exists, skipping update", name)
		return nil
	}

	if gvk.Kind == "PersistentVolumeClaim" {
		log.Printf("DEBUG PVC %s already exists, skipping update", name)
		return nil
	}

	if gvk.Kind == "ClusterRoleBinding" {
		mergeClusterRoleBindingSubjects(existing, obj)
	}

	obj.SetResourceVersion(existing.GetResourceVersion())

	_, err = resourceClient.Update(ctx, obj, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update %s %s: %w", gvk.Kind, name, err)
	}

	return nil
}

func kindToResource(kind string) string {
	mapping := map[string]string{
		"ServiceAccount":        "serviceaccounts",
		"ConfigMap":             "configmaps",
		"Service":               "services",
		"StatefulSet":           "statefulsets",
		"Deployment":            "deployments",
		"Job":                   "jobs",
		"Role":                  "roles",
		"RoleBinding":           "rolebindings",
		"ClusterRole":           "clusterroles",
		"ClusterRoleBinding":    "clusterrolebindings",
		"NetworkPolicy":         "networkpolicies",
		"Secret":                "secrets",
		"PersistentVolumeClaim": "persistentvolumeclaims",
		"Issuer":                "issuers",
		"Certificate":           "certificates",
		"Gateway":               "gateways",
		"GRPCRoute":             "grpcroutes",
		"BackendTLSPolicy":      "backendtlspolicies",
	}

	if resource, ok := mapping[kind]; ok {
		return resource
	}

	log.Printf("DEBUG unknown kind %s, using naive plural", kind)
	return strings.ToLower(kind) + "s"
}

func mergeClusterRoleBindingSubjects(existing, desired *unstructured.Unstructured) {
	existingSubjects, _, _ := unstructured.NestedSlice(existing.Object, "subjects")
	desiredSubjects, _, _ := unstructured.NestedSlice(desired.Object, "subjects")

	seen := make(map[string]bool)
	for _, s := range desiredSubjects {
		sub, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := sub["name"].(string)
		ns, _ := sub["namespace"].(string)
		seen[name+"/"+ns] = true
	}

	for _, s := range existingSubjects {
		sub, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := sub["name"].(string)
		ns, _ := sub["namespace"].(string)
		if !seen[name+"/"+ns] {
			desiredSubjects = append(desiredSubjects, s)
			seen[name+"/"+ns] = true
		}
	}

	_ = unstructured.SetNestedSlice(desired.Object, desiredSubjects, "subjects")
}

func applyConfigHashAnnotation(ctx context.Context, clientset *kubernetes.Clientset, obj *unstructured.Unstructured, namespace string) {
	h := sha256.New()

	cm, err := clientset.CoreV1().ConfigMaps(namespace).Get(ctx, "openshell-gateway-config", metav1.GetOptions{})
	if err == nil {
		keys := make([]string, 0, len(cm.Data))
		for k := range cm.Data {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			h.Write([]byte(k))
			h.Write([]byte(cm.Data[k]))
		}
	} else if !k8serrors.IsNotFound(err) {
		log.Printf("WARN skipping config-hash annotation in %s: failed to get ConfigMap: %v", namespace, err)
		return
	}

	for _, secretName := range []string{"openshell-server-tls", "openshell-gateway-db-credentials"} {
		secret, err := clientset.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
		if err == nil {
			keys := make([]string, 0, len(secret.Data))
			for k := range secret.Data {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				h.Write([]byte(k))
				h.Write(secret.Data[k])
			}
		} else if !k8serrors.IsNotFound(err) {
			log.Printf("WARN skipping config-hash annotation in %s: failed to get Secret %s: %v", namespace, secretName, err)
			return
		}
	}

	hashStr := hex.EncodeToString(h.Sum(nil))

	annotations, _, _ := unstructured.NestedMap(obj.Object, "spec", "template", "metadata", "annotations")
	if annotations == nil {
		annotations = make(map[string]interface{})
	}
	annotations["hypershell.redhat.io/config-hash"] = hashStr
	_ = unstructured.SetNestedMap(obj.Object, annotations, "spec", "template", "metadata", "annotations")
}

func applyOpenShiftOverrides(obj *unstructured.Unstructured) {
	unstructured.RemoveNestedField(obj.Object, "spec", "template", "spec", "securityContext", "fsGroup")

	containers, found, err := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
	if err != nil || !found {
		return
	}
	for i, c := range containers {
		container, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		unstructured.RemoveNestedField(container, "securityContext", "runAsUser")
		containers[i] = container
	}
	_ = unstructured.SetNestedSlice(obj.Object, containers, "spec", "template", "spec", "containers")
}

func reconcileOpenShiftSCC(ctx context.Context, dynamicClient dynamic.Interface, namespace string) error {
	roleBindingGVR := schema.GroupVersionResource{
		Group:    "rbac.authorization.k8s.io",
		Version:  "v1",
		Resource: "rolebindings",
	}
	bindingName := "openshell-sandbox-privileged-scc"
	binding := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "RoleBinding",
			"metadata": map[string]interface{}{
				"name":      bindingName,
				"namespace": namespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":       "openshell",
					"app.kubernetes.io/component":  "gateway",
					"app.kubernetes.io/managed-by": "hypershell-control-plane",
					"hypershell.redhat.io/managed": "true",
				},
			},
			"roleRef": map[string]interface{}{
				"apiGroup": "rbac.authorization.k8s.io",
				"kind":     "ClusterRole",
				"name":     "system:openshift:scc:privileged",
			},
			"subjects": []interface{}{
				map[string]interface{}{
					"kind":      "ServiceAccount",
					"name":      "openshell-gateway-sandbox",
					"namespace": namespace,
				},
			},
		},
	}

	existing, err := dynamicClient.Resource(roleBindingGVR).Namespace(namespace).Get(ctx, bindingName, metav1.GetOptions{})
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return fmt.Errorf("get SCC RoleBinding: %w", err)
		}
		if _, createErr := dynamicClient.Resource(roleBindingGVR).Namespace(namespace).Create(ctx, binding, metav1.CreateOptions{}); createErr != nil {
			return fmt.Errorf("create SCC RoleBinding: %w", createErr)
		}
		log.Printf("INFO created privileged SCC binding for openshell-gateway-sandbox in %s", namespace)
		return nil
	}

	binding.SetResourceVersion(existing.GetResourceVersion())
	if _, err := dynamicClient.Resource(roleBindingGVR).Namespace(namespace).Update(ctx, binding, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update SCC RoleBinding: %w", err)
	}
	return nil
}

func reconcileTrustedCABundle(ctx context.Context, clientset *kubernetes.Clientset, cpNamespace, targetNamespace string) bool {
	if cpNamespace == "" {
		return false
	}

	caConfigMapName := "gateway-trusted-ca"
	sourceCM, err := clientset.CoreV1().ConfigMaps(cpNamespace).Get(ctx, caConfigMapName, metav1.GetOptions{})
	if err != nil {
		return false
	}

	targetCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caConfigMapName,
			Namespace: targetNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "openshell",
				"app.kubernetes.io/component":  "gateway",
				"app.kubernetes.io/managed-by": "hypershell-control-plane",
				"hypershell.redhat.io/managed": "true",
			},
		},
		Data: sourceCM.Data,
	}

	existing, err := clientset.CoreV1().ConfigMaps(targetNamespace).Get(ctx, caConfigMapName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			if _, err := clientset.CoreV1().ConfigMaps(targetNamespace).Create(ctx, targetCM, metav1.CreateOptions{}); err != nil {
				log.Printf("WARN failed to create trusted CA ConfigMap in %s: %v", targetNamespace, err)
				return false
			}
			log.Printf("INFO copied trusted CA ConfigMap to %s", targetNamespace)
			return true
		}
		log.Printf("WARN failed to get trusted CA ConfigMap in %s: %v", targetNamespace, err)
		return false
	}

	targetCM.ResourceVersion = existing.ResourceVersion
	if _, err := clientset.CoreV1().ConfigMaps(targetNamespace).Update(ctx, targetCM, metav1.UpdateOptions{}); err != nil {
		log.Printf("WARN failed to update trusted CA ConfigMap in %s: %v", targetNamespace, err)
		return false
	}
	return true
}

func applyTrustedCAOverrides(obj *unstructured.Unstructured) {
	volumes, found, _ := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "volumes")
	if !found {
		return
	}

	caVolume := map[string]interface{}{
		"name": "trusted-ca",
		"configMap": map[string]interface{}{
			"name": "gateway-trusted-ca",
			"items": []interface{}{
				map[string]interface{}{
					"key":  "ca-bundle.crt",
					"path": "ca-bundle.crt",
				},
			},
		},
	}
	volumes = append(volumes, caVolume)
	_ = unstructured.SetNestedSlice(obj.Object, volumes, "spec", "template", "spec", "volumes")

	containers, found, _ := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
	if !found {
		return
	}
	for i, c := range containers {
		container, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		name, _, _ := unstructured.NestedString(container, "name")
		if name != "openshell-gateway" {
			continue
		}

		volumeMounts, _, _ := unstructured.NestedSlice(container, "volumeMounts")
		volumeMounts = append(volumeMounts, map[string]interface{}{
			"name":      "trusted-ca",
			"mountPath": "/etc/pki/tls/certs/ca-bundle.crt",
			"subPath":   "ca-bundle.crt",
			"readOnly":  true,
		})
		_ = unstructured.SetNestedSlice(container, volumeMounts, "volumeMounts")

		env, _, _ := unstructured.NestedSlice(container, "env")
		env = append(env, map[string]interface{}{
			"name":  "SSL_CERT_FILE",
			"value": "/etc/pki/tls/certs/ca-bundle.crt",
		})
		_ = unstructured.SetNestedSlice(container, env, "env")

		containers[i] = container
	}
	_ = unstructured.SetNestedSlice(obj.Object, containers, "spec", "template", "spec", "containers")
}

func reconcileDatabaseCredentials(ctx context.Context, clientset *kubernetes.Clientset, namespace string, dbImage string) error {
	secretName := "openshell-gateway-db-credentials"
	_, err := clientset.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err == nil {
		log.Printf("DEBUG database credentials secret %s already exists in %s, skipping", secretName, namespace)
		return nil
	}
	if !k8serrors.IsNotFound(err) {
		return fmt.Errorf("get database credentials secret: %w", err)
	}

	passwordBytes := make([]byte, 32)
	if _, err := rand.Read(passwordBytes); err != nil {
		return fmt.Errorf("generate database password: %w", err)
	}
	password := hex.EncodeToString(passwordBytes)

	dbUser := "openshell"
	dbName := "openshell"
	dbHost := "openshell-gateway-db"
	dbURL := fmt.Sprintf("postgresql://%s:%s@%s:5432/%s?sslmode=disable",
		dbUser, url.QueryEscape(password), dbHost, dbName)

	userKey, passKey, dbKey := postgresEnvKeys(dbImage)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "openshell",
				"app.kubernetes.io/component":  "database",
				"app.kubernetes.io/managed-by": "hypershell-control-plane",
				"hypershell.redhat.io/managed": "true",
			},
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			userKey: dbUser,
			passKey: password,
			dbKey:   dbName,
			"url":   dbURL,
		},
	}

	if _, err := clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("create database credentials secret: %w", err)
	}

	log.Printf("INFO created database credentials secret %s in %s (password length=%d)", secretName, namespace, len(password))
	return nil
}

func reconcileKeycloakClient(ctx context.Context, opts ReconcileOpts, nsConfig *NamespaceConfig) error {
	kc := keycloak.NewClient(
		opts.Keycloak.ServerURL,
		opts.Keycloak.Realm,
		opts.Keycloak.ClientID,
		opts.Keycloak.ClientSecret,
	)

	if opts.GatewayName == "" {
		return fmt.Errorf("gateway name is required for keycloak provisioning")
	}
	if opts.GatewayID == "" {
		return fmt.Errorf("gateway ID is required for keycloak provisioning")
	}
	kcClientID := fmt.Sprintf("%s-%s", opts.GatewayName, opts.GatewayID)

	existingUUID, err := kc.GetClientUUID(ctx, kcClientID)
	if err != nil {
		return fmt.Errorf("check existing keycloak client: %w", err)
	}

	if existingUUID != "" {
		log.Printf("INFO keycloak client %s already exists (uuid=%s), skipping provisioning", kcClientID, existingUUID)
	} else {
		clientUUID, err := kc.ProvisionGatewayClient(ctx, kcClientID)
		if err != nil {
			return fmt.Errorf("provision keycloak client %s: %w", kcClientID, err)
		}
		log.Printf("INFO provisioned keycloak client %s (uuid=%s)", kcClientID, clientUUID)
	}

	oidcConfig := OIDCConfig{
		Issuer:     kc.Issuer(),
		ClientID:   kcClientID,
		Audience:   kcClientID,
		JwksTTL:    3600,
		RolesClaim: "hypershell.roles",
		AdminRole:  "openshell-admin",
		UserRole:   "openshell-user",
	}
	// The Keycloak Admin API server URL must be reachable in-cluster, but the
	// gateway's client-facing issuer (consumed by the gateway pod, console, and
	// CLI) may need to be a separately reachable URL. When GATEWAY_OIDC_ISSUER_URL
	// is set it overrides the admin-derived issuer; it MUST equal Keycloak's
	// KC_HOSTNAME so the token `iss` claim validates. Unset preserves 98's default.
	if issuerURL := os.Getenv("GATEWAY_OIDC_ISSUER_URL"); issuerURL != "" {
		oidcConfig.Issuer = issuerURL
	}
	nsConfig.Gateway.OIDC = oidcConfig

	if opts.UpdateOIDC != nil {
		oidcJSON, err := json.Marshal(oidcConfig)
		if err != nil {
			return fmt.Errorf("marshal oidc config: %w", err)
		}
		if err := opts.UpdateOIDC(ctx, string(oidcJSON)); err != nil {
			log.Printf("WARN failed to persist oidc config for %s: %v", kcClientID, err)
		}
	}

	return nil
}

func rotateDatabaseCredentials(ctx context.Context, clientset *kubernetes.Clientset, namespace string, dbImage string, rotateTimestamp string) error {
	secretName := "openshell-gateway-db-credentials"
	existing, err := clientset.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get database credentials secret for rotation: %w", err)
	}

	lastRotation := existing.Annotations["hypershell.redhat.io/last-db-rotation"]
	if lastRotation == rotateTimestamp {
		log.Printf("DEBUG database credentials in %s already rotated at %s, skipping", namespace, rotateTimestamp)
		return nil
	}

	passwordBytes := make([]byte, 32)
	if _, err := rand.Read(passwordBytes); err != nil {
		return fmt.Errorf("generate new database password: %w", err)
	}
	newPassword := hex.EncodeToString(passwordBytes)

	userKey, passKey, _ := postgresEnvKeys(dbImage)
	dbUser := string(existing.Data[userKey])
	if dbUser == "" {
		dbUser = "openshell"
	}

	dbHost := fmt.Sprintf("openshell-gateway-db.%s.svc.cluster.local", namespace)
	connStr := fmt.Sprintf("postgresql://%s:%s@%s:5432/openshell?sslmode=disable&connect_timeout=10",
		dbUser, url.QueryEscape(string(existing.Data[passKey])), dbHost)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return fmt.Errorf("open database connection for rotation: %w", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.ExecContext(ctx, fmt.Sprintf("ALTER ROLE %s WITH PASSWORD %s", pq.QuoteIdentifier(dbUser), pq.QuoteLiteral(newPassword))); err != nil {
		return fmt.Errorf("ALTER ROLE during credential rotation: %w", err)
	}
	log.Printf("INFO executed ALTER ROLE for user %s in %s", dbUser, namespace)

	dbName := "openshell"
	dbURL := fmt.Sprintf("postgresql://%s:%s@openshell-gateway-db:5432/%s?sslmode=disable",
		dbUser, url.QueryEscape(newPassword), dbName)

	existing.Data[passKey] = []byte(newPassword)
	existing.Data["url"] = []byte(dbURL)
	if existing.Annotations == nil {
		existing.Annotations = make(map[string]string)
	}
	existing.Annotations["hypershell.redhat.io/last-db-rotation"] = rotateTimestamp

	if _, err := clientset.CoreV1().Secrets(namespace).Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update database credentials secret after rotation: %w", err)
	}

	log.Printf("INFO rotated database credentials in %s (timestamp=%s)", namespace, rotateTimestamp)
	return nil
}

func isRHELPostgres(image string) bool {
	return strings.Contains(image, "rhel") && strings.Contains(image, "postgresql-")
}

func postgresEnvKeys(image string) (userKey, passKey, dbKey string) {
	if isRHELPostgres(image) {
		return "POSTGRESQL_USER", "POSTGRESQL_PASSWORD", "POSTGRESQL_DATABASE"
	}
	return "POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DB"
}

func postgresDataPath(image string) string {
	if isRHELPostgres(image) {
		return "/var/lib/pgsql/data"
	}
	return "/var/lib/postgresql/data"
}

func postgresPGDataPath(image string) string {
	if isRHELPostgres(image) {
		return "/var/lib/pgsql/data"
	}
	return "/var/lib/postgresql/data/pgdata"
}

// reconcileCredentialKEK uses create-or-skip (not update-or-create) because
// replacing an existing key would render all previously encrypted credentials
// unrecoverable.
func reconcileCredentialKEK(ctx context.Context, clientset *kubernetes.Clientset, namespace string) error {
	secretName := "openshell-gateway-credential-kek"
	_, err := clientset.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err == nil {
		log.Printf("DEBUG credential KEK secret %s already exists in %s, skipping", secretName, namespace)
		return nil
	}
	if !k8serrors.IsNotFound(err) {
		return fmt.Errorf("get credential KEK secret: %w", err)
	}

	kekBytes := make([]byte, 32)
	if _, err := rand.Read(kekBytes); err != nil {
		return fmt.Errorf("generate credential KEK: %w", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "openshell",
				"app.kubernetes.io/component":  "gateway",
				"app.kubernetes.io/managed-by": "hypershell-control-plane",
				"hypershell.redhat.io/managed": "true",
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"key-encryption-key": []byte(base64.StdEncoding.EncodeToString(kekBytes)),
		},
	}

	if _, err := clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("create credential KEK secret: %w", err)
	}

	log.Printf("INFO created credential KEK secret %s in %s", secretName, namespace)
	return nil
}

func reconcileCredentialDriverResources(
	ctx context.Context,
	dynamicClient dynamic.Interface,
	clientset *kubernetes.Clientset,
	nsConfig NamespaceConfig,
) error {
	driver := nsConfig.Gateway.CredentialDriver
	if driver.Type == "kubernetes-secrets" {
		credNS := nsConfig.Name
		if driver.KubernetesSecrets != nil && driver.KubernetesSecrets.Namespace != "" {
			credNS = driver.KubernetesSecrets.Namespace
		}
		if err := reconcileCredentialSecretsRBAC(ctx, dynamicClient, nsConfig.Name, credNS); err != nil {
			return fmt.Errorf("reconcile credential secrets RBAC: %w", err)
		}
	} else {
		deleteCredentialSecretsRBAC(ctx, dynamicClient, nsConfig.Name)
	}
	return nil
}

func deleteCredentialSecretsRBAC(ctx context.Context, dynamicClient dynamic.Interface, namespace string) {
	roleGVR := schema.GroupVersionResource{
		Group:    "rbac.authorization.k8s.io",
		Version:  "v1",
		Resource: "roles",
	}
	roleBindingGVR := schema.GroupVersionResource{
		Group:    "rbac.authorization.k8s.io",
		Version:  "v1",
		Resource: "rolebindings",
	}

	name := "openshell-gateway-credential-secrets"
	if err := dynamicClient.Resource(roleBindingGVR).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
		log.Printf("WARN failed to delete credential secrets RoleBinding in %s: %v", namespace, err)
	}
	if err := dynamicClient.Resource(roleGVR).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
		log.Printf("WARN failed to delete credential secrets Role in %s: %v", namespace, err)
	}
}

func reconcileCredentialSecretsRBAC(
	ctx context.Context,
	dynamicClient dynamic.Interface,
	gatewayNamespace, credentialNamespace string,
) error {
	managedLabels := map[string]interface{}{
		"app.kubernetes.io/name":       "openshell",
		"app.kubernetes.io/component":  "gateway",
		"app.kubernetes.io/managed-by": "hypershell-control-plane",
		"hypershell.redhat.io/managed": "true",
	}

	role := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "Role",
			"metadata": map[string]interface{}{
				"name":      "openshell-gateway-credential-secrets",
				"namespace": credentialNamespace,
				"labels":    managedLabels,
			},
			"rules": []interface{}{
				map[string]interface{}{
					"apiGroups": []interface{}{""},
					"resources": []interface{}{"secrets"},
					"verbs":     []interface{}{"get", "create", "patch", "delete"},
				},
			},
		},
	}

	roleBinding := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "RoleBinding",
			"metadata": map[string]interface{}{
				"name":      "openshell-gateway-credential-secrets",
				"namespace": credentialNamespace,
				"labels":    managedLabels,
			},
			"roleRef": map[string]interface{}{
				"apiGroup": "rbac.authorization.k8s.io",
				"kind":     "Role",
				"name":     "openshell-gateway-credential-secrets",
			},
			"subjects": []interface{}{
				map[string]interface{}{
					"kind":      "ServiceAccount",
					"name":      "openshell-gateway",
					"namespace": gatewayNamespace,
				},
			},
		},
	}

	if err := reconcileResource(ctx, dynamicClient, role); err != nil {
		return fmt.Errorf("reconcile credential secrets Role: %w", err)
	}
	if err := reconcileResource(ctx, dynamicClient, roleBinding); err != nil {
		return fmt.Errorf("reconcile credential secrets RoleBinding: %w", err)
	}

	log.Printf("INFO reconciled credential secrets RBAC in %s for gateway in %s", credentialNamespace, gatewayNamespace)
	return nil
}

func DetectOpenShift(clientset *kubernetes.Clientset) bool {
	_, resources, err := clientset.Discovery().ServerGroupsAndResources()
	if err != nil {
		log.Printf("WARN failed to discover API groups, assuming non-OpenShift: %v", err)
		return false
	}
	for _, list := range resources {
		if strings.HasPrefix(list.GroupVersion, "route.openshift.io/") {
			return true
		}
	}
	return false
}

func DetectCertManager(clientset *kubernetes.Clientset) bool {
	_, resources, err := clientset.Discovery().ServerGroupsAndResources()
	if err != nil {
		log.Printf("WARN failed to discover API groups for cert-manager detection: %v", err)
		return false
	}
	for _, list := range resources {
		if strings.HasPrefix(list.GroupVersion, "cert-manager.io/") {
			return true
		}
	}
	return false
}

func DetectGatewayAPI(clientset *kubernetes.Clientset) bool {
	_, resources, err := clientset.Discovery().ServerGroupsAndResources()
	if err != nil {
		log.Printf("WARN failed to discover API groups for Gateway API detection: %v", err)
		return false
	}
	for _, list := range resources {
		if list.GroupVersion == "gateway.networking.k8s.io/v1" {
			for _, r := range list.APIResources {
				if r.Kind == "GRPCRoute" {
					return true
				}
			}
		}
	}
	return false
}

func gatewayIngressNamespace() string {
	if ns := os.Getenv("GATEWAY_API_GATEWAY_NAMESPACE"); ns != "" {
		return ns
	}
	return "openshift-ingress"
}

func gatewayIngressName() string {
	if name := os.Getenv("GATEWAY_API_GATEWAY_NAME"); name != "" {
		return name
	}
	return ""
}

func reconcileGatewayAPIResources(ctx context.Context, dynamicClient dynamic.Interface, clientset *kubernetes.Clientset, nsConfig NamespaceConfig, opts ReconcileOpts) error {
	namespace := nsConfig.Name
	routeConfig := nsConfig.Gateway.Route

	gwName := gatewayIngressName()
	if gwName == "" {
		log.Printf("WARN GATEWAY_API_GATEWAY_NAME is required -- set it to the name of a pre-existing Gateway resource")
		return fmt.Errorf("GATEWAY_API_GATEWAY_NAME is required")
	}
	gwNS := gatewayIngressNamespace()

	// Derive the external hostname through the Gateway Exposure adapter's shared
	// helper so the hostname baked into the GRPCRoute cannot drift from the
	// address published through the port.
	hostname, ok := exposure.DeriveGatewayAPIHost(namespace, routeConfig.Host)
	if !ok {
		log.Printf("WARN cannot derive GRPCRoute hostname: GATEWAY_API_BASE_DOMAIN not set")
		return nil
	}

	// Publish the deterministic route address through the Gateway Exposure port.
	// The hostname is known before the shared Gateway reports Accepted/Programmed,
	// so the connection command is available to the CLI and console while the
	// gateway finishes provisioning. Readiness is reflected separately by the
	// Gateway phase.
	if opts.Exposure != nil && opts.UpdateRouteAddress != nil {
		routeAddress, err := opts.Exposure.ResolveAddress(ctx, exposure.Request{Namespace: namespace, Host: routeConfig.Host})
		if err != nil {
			log.Printf("WARN failed to resolve routeAddress for gateway in %s: %v", namespace, err)
		} else if routeAddress != "" {
			if err := opts.UpdateRouteAddress(ctx, routeAddress); err != nil {
				log.Printf("WARN failed to publish routeAddress %s for gateway in %s: %v", routeAddress, namespace, err)
			} else {
				log.Printf("INFO published routeAddress %s for gateway in %s", routeAddress, namespace)
			}
		}
	}

	log.Printf("INFO using Gateway %s/%s for tenant %s", gwNS, gwName, namespace)

	parentRef := map[string]interface{}{
		"name":        gwName,
		"namespace":   gwNS,
		"sectionName": "grpc",
	}

	grpcRoute := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "GRPCRoute",
			"metadata": map[string]interface{}{
				"name":      "openshell-gateway",
				"namespace": namespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":       "openshell",
					"app.kubernetes.io/component":  "gateway",
					"app.kubernetes.io/managed-by": "hypershell-control-plane",
					"hypershell.redhat.io/managed": "true",
				},
			},
			"spec": map[string]interface{}{
				"parentRefs": []interface{}{parentRef},
				"hostnames":  []interface{}{hostname},
				"rules": []interface{}{
					map[string]interface{}{
						"backendRefs": []interface{}{
							map[string]interface{}{
								"name": "openshell-gateway",
								"port": int64(8080),
							},
						},
					},
				},
			},
		},
	}
	if err := reconcileResource(ctx, dynamicClient, grpcRoute); err != nil {
		return fmt.Errorf("reconcile GRPCRoute: %w", err)
	}

	if err := waitForSecret(ctx, clientset, namespace, "openshell-server-tls", 60*time.Second); err != nil {
		return fmt.Errorf("wait for server TLS secret in %s: %w", namespace, err)
	}

	caData := ""
	tlsSecret, err := clientset.CoreV1().Secrets(namespace).Get(ctx, "openshell-server-tls", metav1.GetOptions{})
	if err == nil {
		if ca, ok := tlsSecret.Data["ca.crt"]; ok {
			caData = string(ca)
		}
	}

	if caData != "" {
		backendCA := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "openshell-backend-ca",
				Namespace: namespace,
				Labels: map[string]string{
					"app.kubernetes.io/name":       "openshell",
					"app.kubernetes.io/component":  "gateway",
					"app.kubernetes.io/managed-by": "hypershell-control-plane",
					"hypershell.redhat.io/managed": "true",
				},
			},
			Data: map[string]string{
				"ca.crt": caData,
			},
		}

		existing, err := clientset.CoreV1().ConfigMaps(namespace).Get(ctx, "openshell-backend-ca", metav1.GetOptions{})
		if err != nil {
			if k8serrors.IsNotFound(err) {
				if _, err := clientset.CoreV1().ConfigMaps(namespace).Create(ctx, backendCA, metav1.CreateOptions{}); err != nil {
					log.Printf("WARN failed to create backend CA ConfigMap: %v", err)
				}
			}
		} else {
			backendCA.ResourceVersion = existing.ResourceVersion
			if _, err := clientset.CoreV1().ConfigMaps(namespace).Update(ctx, backendCA, metav1.UpdateOptions{}); err != nil {
				log.Printf("WARN failed to update backend CA ConfigMap: %v", err)
			}
		}

		btlsPolicy := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "gateway.networking.k8s.io/v1",
				"kind":       "BackendTLSPolicy",
				"metadata": map[string]interface{}{
					"name":      "openshell-gateway",
					"namespace": namespace,
					"labels": map[string]interface{}{
						"app.kubernetes.io/name":       "openshell",
						"app.kubernetes.io/component":  "gateway",
						"app.kubernetes.io/managed-by": "hypershell-control-plane",
						"hypershell.redhat.io/managed": "true",
					},
				},
				"spec": map[string]interface{}{
					"targetRefs": []interface{}{
						map[string]interface{}{
							"group": "",
							"kind":  "Service",
							"name":  "openshell-gateway",
						},
					},
					"validation": map[string]interface{}{
						"caCertificateRefs": []interface{}{
							map[string]interface{}{
								"group": "",
								"kind":  "ConfigMap",
								"name":  "openshell-backend-ca",
							},
						},
						"hostname": fmt.Sprintf("openshell-gateway.%s.svc.cluster.local", namespace),
					},
				},
			},
		}
		if err := reconcileResource(ctx, dynamicClient, btlsPolicy); err != nil {
			log.Printf("WARN failed to reconcile BackendTLSPolicy (may require OpenShift 4.22+): %v", err)
		}
	}

	// Build the router → gateway NetworkPolicy unless dev has opted out (Kind's
	// out-of-cluster proxy has a source IP no selector can match, so the policy
	// would blackhole gateway ingress). Restrict source to the namespace hosting
	// the shared Gateway so only the admin-provisioned proxy can reach the ports.
	if opts.SkipNetworkPolicies {
		logNetworkPoliciesDisabled()
	} else {
		ingressRule := map[string]interface{}{
			"ports": []interface{}{
				map[string]interface{}{
					"port":     int64(8080),
					"protocol": "TCP",
				},
				map[string]interface{}{
					"port":     int64(8081),
					"protocol": "TCP",
				},
			},
			"from": []interface{}{
				map[string]interface{}{
					"namespaceSelector": map[string]interface{}{
						"matchLabels": map[string]interface{}{
							"kubernetes.io/metadata.name": gwNS,
						},
					},
				},
			},
		}

		routerNetpol := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "networking.k8s.io/v1",
				"kind":       "NetworkPolicy",
				"metadata": map[string]interface{}{
					"name":      "openshell-gateway-allow-router",
					"namespace": namespace,
					"labels": map[string]interface{}{
						"app.kubernetes.io/name":       "openshell",
						"app.kubernetes.io/component":  "gateway",
						"app.kubernetes.io/managed-by": "hypershell-control-plane",
						"hypershell.redhat.io/managed": "true",
					},
				},
				"spec": map[string]interface{}{
					"podSelector": map[string]interface{}{
						"matchLabels": map[string]interface{}{
							"app.kubernetes.io/instance": "openshell-gateway",
							"app.kubernetes.io/name":     "openshell",
						},
					},
					"policyTypes": []interface{}{"Ingress"},
					"ingress":     []interface{}{ingressRule},
				},
			},
		}
		if err := reconcileResource(ctx, dynamicClient, routerNetpol); err != nil {
			log.Printf("WARN failed to reconcile router NetworkPolicy: %v", err)
		}
	}

	// The route address is published deterministically at the top of this
	// function, so no readiness-gated discovery is required here.

	log.Printf("INFO Gateway API resources reconciled in namespace %s (hostname=%s)", namespace, hostname)
	return nil
}

func reconcileCertManagerResources(ctx context.Context, dynamicClient dynamic.Interface, nsConfig NamespaceConfig) error {
	namespace := nsConfig.Name
	dnsNames := nsConfig.Gateway.ServerDnsNames

	selfSignedIssuer := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cert-manager.io/v1",
			"kind":       "Issuer",
			"metadata": map[string]interface{}{
				"name":      "openshell-selfsigned",
				"namespace": namespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":       "openshell",
					"app.kubernetes.io/component":  "gateway",
					"app.kubernetes.io/managed-by": "hypershell-control-plane",
					"hypershell.redhat.io/managed": "true",
				},
			},
			"spec": map[string]interface{}{
				"selfSigned": map[string]interface{}{},
			},
		},
	}
	if err := reconcileResource(ctx, dynamicClient, selfSignedIssuer); err != nil {
		return fmt.Errorf("reconcile self-signed issuer: %w", err)
	}

	caCert := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cert-manager.io/v1",
			"kind":       "Certificate",
			"metadata": map[string]interface{}{
				"name":      "openshell-ca",
				"namespace": namespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":       "openshell",
					"app.kubernetes.io/component":  "gateway",
					"app.kubernetes.io/managed-by": "hypershell-control-plane",
					"hypershell.redhat.io/managed": "true",
				},
			},
			"spec": map[string]interface{}{
				"isCA":       true,
				"commonName": "openshell-ca",
				"secretName": "openshell-ca-tls",
				"privateKey": map[string]interface{}{
					"algorithm": "ECDSA",
					"size":      int64(256),
				},
				"issuerRef": map[string]interface{}{
					"name":  "openshell-selfsigned",
					"kind":  "Issuer",
					"group": "cert-manager.io",
				},
			},
		},
	}
	if err := reconcileResource(ctx, dynamicClient, caCert); err != nil {
		return fmt.Errorf("reconcile CA certificate: %w", err)
	}

	caIssuer := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cert-manager.io/v1",
			"kind":       "Issuer",
			"metadata": map[string]interface{}{
				"name":      "openshell-ca-issuer",
				"namespace": namespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":       "openshell",
					"app.kubernetes.io/component":  "gateway",
					"app.kubernetes.io/managed-by": "hypershell-control-plane",
					"hypershell.redhat.io/managed": "true",
				},
			},
			"spec": map[string]interface{}{
				"ca": map[string]interface{}{
					"secretName": "openshell-ca-tls",
				},
			},
		},
	}
	if err := reconcileResource(ctx, dynamicClient, caIssuer); err != nil {
		return fmt.Errorf("reconcile CA issuer: %w", err)
	}

	dnsNamesInterface := make([]interface{}, len(dnsNames))
	for i, d := range dnsNames {
		dnsNamesInterface[i] = d
	}

	serverCert := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cert-manager.io/v1",
			"kind":       "Certificate",
			"metadata": map[string]interface{}{
				"name":      "openshell-server",
				"namespace": namespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":       "openshell",
					"app.kubernetes.io/component":  "gateway",
					"app.kubernetes.io/managed-by": "hypershell-control-plane",
					"hypershell.redhat.io/managed": "true",
				},
			},
			"spec": map[string]interface{}{
				"secretName": "openshell-server-tls",
				"dnsNames":   dnsNamesInterface,
				"issuerRef": map[string]interface{}{
					"name":  "openshell-ca-issuer",
					"kind":  "Issuer",
					"group": "cert-manager.io",
				},
			},
		},
	}
	if err := reconcileResource(ctx, dynamicClient, serverCert); err != nil {
		return fmt.Errorf("reconcile server certificate: %w", err)
	}

	clientCert := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cert-manager.io/v1",
			"kind":       "Certificate",
			"metadata": map[string]interface{}{
				"name":      "openshell-client",
				"namespace": namespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":       "openshell",
					"app.kubernetes.io/component":  "gateway",
					"app.kubernetes.io/managed-by": "hypershell-control-plane",
					"hypershell.redhat.io/managed": "true",
				},
			},
			"spec": map[string]interface{}{
				"secretName": "openshell-client-tls",
				"commonName": "openshell-client",
				"issuerRef": map[string]interface{}{
					"name":  "openshell-ca-issuer",
					"kind":  "Issuer",
					"group": "cert-manager.io",
				},
			},
		},
	}
	if err := reconcileResource(ctx, dynamicClient, clientCert); err != nil {
		return fmt.Errorf("reconcile client certificate: %w", err)
	}

	log.Printf("INFO cert-manager resources reconciled in namespace %s", namespace)
	return nil
}
