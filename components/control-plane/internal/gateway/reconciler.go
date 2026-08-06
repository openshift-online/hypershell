package gateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	corev1 "k8s.io/api/core/v1"
)

func ReconcileGateway(
	ctx context.Context,
	dynamicClient dynamic.Interface,
	clientset *kubernetes.Clientset,
	nsConfig NamespaceConfig,
	manifests map[string][]*unstructured.Unstructured,
	opts ReconcileOpts,
) error {
	defaultImage := os.Getenv("OPENSHELL_GATEWAY_IMAGE")
	if defaultImage == "" {
		defaultImage = "ghcr.io/nvidia/openshell/gateway:0.0.92"
	}

	if !namespaceExists(ctx, clientset, nsConfig.Name) {
		if err := createNamespace(ctx, clientset, nsConfig.Name); err != nil {
			return fmt.Errorf("create namespace %s: %w", nsConfig.Name, err)
		}
	}

	if err := ValidateGatewayConfig(nsConfig.Gateway); err != nil {
		return fmt.Errorf("invalid gateway configuration: %w", err)
	}

	if err := reconcileDatabaseCredentials(ctx, clientset, nsConfig.Name); err != nil {
		return fmt.Errorf("reconcile database credentials in %s: %w", nsConfig.Name, err)
	}

	if opts.HasCertManager {
		if err := reconcileCertManagerResources(ctx, dynamicClient, nsConfig); err != nil {
			return fmt.Errorf("reconcile cert-manager resources in %s: %w", nsConfig.Name, err)
		}
	} else {
		log.Printf("WARN cert-manager not available, TLS certificates must be provisioned by certgen job in %s", nsConfig.Name)
	}

	hasTrustedCA := reconcileTrustedCABundle(ctx, clientset, opts.ControlPlaneNamespace, nsConfig.Name)

	if err := deployGateway(ctx, dynamicClient, nsConfig, manifests, defaultImage, opts, hasTrustedCA); err != nil {
		return fmt.Errorf("deploy gateway in %s: %w", nsConfig.Name, err)
	}

	if opts.IsOpenShift {
		if err := reconcileOpenShiftSCC(ctx, dynamicClient, nsConfig.Name); err != nil {
			log.Printf("WARN failed to reconcile OpenShift SCC binding in %s: %v", nsConfig.Name, err)
		}
		if err := reconcilePassthroughRoute(ctx, dynamicClient, nsConfig); err != nil {
			log.Printf("WARN failed to reconcile passthrough Route in %s: %v", nsConfig.Name, err)
		}
		if err := reconcileRouterNetworkPolicy(ctx, dynamicClient, nsConfig.Name); err != nil {
			log.Printf("WARN failed to reconcile router NetworkPolicy in %s: %v", nsConfig.Name, err)
		}
	}

	if opts.HasGatewayAPI {
		if err := reconcileGatewayAPIResources(ctx, dynamicClient, clientset, nsConfig); err != nil {
			log.Printf("WARN failed to reconcile Gateway API resources in %s: %v", nsConfig.Name, err)
		}
	}

	log.Printf("INFO gateway reconciled in namespace %s", nsConfig.Name)
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
	nsConfig NamespaceConfig,
	manifests map[string][]*unstructured.Unstructured,
	defaultImage string,
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
			obj, err := ApplyManifestToNamespace(manifest, nsConfig.Name, nsConfig.Gateway, defaultImage)
			if err != nil {
				return fmt.Errorf("apply substitutions for %s: %w", filename, err)
			}

			if err := ApplyConfigOverrides(obj, nsConfig.Gateway); err != nil {
				return fmt.Errorf("apply config overrides for %s: %w", filename, err)
			}

			if err := ApplyDatabaseOverrides(obj, nsConfig.Gateway.Database); err != nil {
				return fmt.Errorf("apply database overrides for %s: %w", filename, err)
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
	}

	return nil
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
		"Route":                 "routes",
		"PersistentVolumeClaim": "persistentvolumeclaims",
		"Issuer":                "issuers",
		"Certificate":           "certificates",
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

func reconcilePassthroughRoute(ctx context.Context, dynamicClient dynamic.Interface, nsConfig NamespaceConfig) error {
	osRouteGVR := schema.GroupVersionResource{
		Group:    "route.openshift.io",
		Version:  "v1",
		Resource: "routes",
	}

	namespace := nsConfig.Name
	routeName := "openshell-gateway-grpc"

	hostname := nsConfig.Gateway.ExternalDns
	if hostname == "" {
		hostname = fmt.Sprintf("openshell-gateway-%s.apps.openshiftapps.com", namespace)
	}

	routeSpec := map[string]interface{}{
		"port": map[string]interface{}{
			"targetPort": "grpc",
		},
		"tls": map[string]interface{}{
			"termination": "passthrough",
		},
		"to": map[string]interface{}{
			"kind":   "Service",
			"name":   "openshell-gateway",
			"weight": int64(100),
		},
	}

	if hostname != "" {
		routeSpec["host"] = hostname
	}

	route := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "route.openshift.io/v1",
			"kind":       "Route",
			"metadata": map[string]interface{}{
				"name":      routeName,
				"namespace": namespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":       "openshell",
					"app.kubernetes.io/component":  "gateway",
					"app.kubernetes.io/managed-by": "hypershell-control-plane",
					"hypershell.redhat.io/managed": "true",
				},
				"annotations": map[string]interface{}{
					"haproxy.router.openshift.io/timeout": "3600s",
				},
			},
			"spec": routeSpec,
		},
	}

	existing, err := dynamicClient.Resource(osRouteGVR).Namespace(namespace).Get(ctx, routeName, metav1.GetOptions{})
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return fmt.Errorf("get Route: %w", err)
		}
		if _, createErr := dynamicClient.Resource(osRouteGVR).Namespace(namespace).Create(ctx, route, metav1.CreateOptions{}); createErr != nil {
			return fmt.Errorf("create Route: %w", createErr)
		}
		log.Printf("INFO created passthrough Route %s in %s", routeName, namespace)
		return nil
	}

	route.SetResourceVersion(existing.GetResourceVersion())
	if _, err := dynamicClient.Resource(osRouteGVR).Namespace(namespace).Update(ctx, route, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update Route: %w", err)
	}
	return nil
}

func reconcileRouterNetworkPolicy(ctx context.Context, dynamicClient dynamic.Interface, namespace string) error {
	netpolGVR := schema.GroupVersionResource{
		Group:    "networking.k8s.io",
		Version:  "v1",
		Resource: "networkpolicies",
	}

	policyName := "openshell-gateway-allow-router"
	policy := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.k8s.io/v1",
			"kind":       "NetworkPolicy",
			"metadata": map[string]interface{}{
				"name":      policyName,
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
				"ingress": []interface{}{
					map[string]interface{}{
						"from": []interface{}{
							map[string]interface{}{
								"namespaceSelector": map[string]interface{}{
									"matchLabels": map[string]interface{}{
										"kubernetes.io/metadata.name": "openshift-ingress",
									},
								},
							},
						},
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
					},
				},
			},
		},
	}

	existing, err := dynamicClient.Resource(netpolGVR).Namespace(namespace).Get(ctx, policyName, metav1.GetOptions{})
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return fmt.Errorf("get router NetworkPolicy: %w", err)
		}
		if _, createErr := dynamicClient.Resource(netpolGVR).Namespace(namespace).Create(ctx, policy, metav1.CreateOptions{}); createErr != nil {
			return fmt.Errorf("create router NetworkPolicy: %w", createErr)
		}
		log.Printf("INFO created router NetworkPolicy %s in %s", policyName, namespace)
		return nil
	}

	policy.SetResourceVersion(existing.GetResourceVersion())
	if _, err := dynamicClient.Resource(netpolGVR).Namespace(namespace).Update(ctx, policy, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update router NetworkPolicy: %w", err)
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

func reconcileDatabaseCredentials(ctx context.Context, clientset *kubernetes.Clientset, namespace string) error {
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
			"POSTGRESQL_USER":     dbUser,
			"POSTGRESQL_PASSWORD": password,
			"POSTGRESQL_DATABASE": dbName,
			"url":                 dbURL,
		},
	}

	if _, err := clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("create database credentials secret: %w", err)
	}

	log.Printf("INFO created database credentials secret %s in %s (password length=%d)", secretName, namespace, len(password))
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

func reconcileGatewayAPIResources(ctx context.Context, dynamicClient dynamic.Interface, clientset *kubernetes.Clientset, nsConfig NamespaceConfig) error {
	namespace := nsConfig.Name
	routeConfig := nsConfig.Gateway.Route

	if !routeConfig.Enabled {
		return nil
	}

	gatewayName := os.Getenv("GATEWAY_API_GATEWAY_NAME")
	if gatewayName == "" {
		gatewayName = "hsgw"
	}
	gatewayNamespace := os.Getenv("GATEWAY_API_GATEWAY_NAMESPACE")
	if gatewayNamespace == "" {
		gatewayNamespace = "openshift-ingress"
	}

	hostname := routeConfig.Host
	if hostname == "" {
		baseDomain := os.Getenv("GATEWAY_API_BASE_DOMAIN")
		if baseDomain == "" {
			log.Printf("WARN cannot derive GRPCRoute hostname: GATEWAY_API_BASE_DOMAIN not set")
			return nil
		}
		hostname = fmt.Sprintf("openshell-gateway-%s.hsgw.%s", namespace, baseDomain)
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
				"parentRefs": []interface{}{
					map[string]interface{}{
						"name":      gatewayName,
						"namespace": gatewayNamespace,
					},
				},
				"hostnames": []interface{}{hostname},
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
				"apiVersion": "gateway.networking.k8s.io/v1alpha3",
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
