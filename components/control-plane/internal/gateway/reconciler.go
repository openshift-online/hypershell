package gateway

import (
	"context"
	"fmt"
	"log"
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

	if err := deployGateway(ctx, dynamicClient, nsConfig, manifests, defaultImage, opts); err != nil {
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
				"app.kubernetes.io/managed-by": "hypershell",
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
) error {
	order := []string{
		"rbac.yaml",
		"serviceaccount.yaml",
		"configmap.yaml",
		"certgen-job.yaml",
		"service.yaml",
		"statefulset.yaml",
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

			if opts.IsOpenShift && (obj.GetKind() == "StatefulSet" || obj.GetKind() == "Deployment") {
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
		"ServiceAccount":      "serviceaccounts",
		"ConfigMap":           "configmaps",
		"Service":             "services",
		"StatefulSet":         "statefulsets",
		"Deployment":          "deployments",
		"Job":                 "jobs",
		"Role":                "roles",
		"RoleBinding":         "rolebindings",
		"ClusterRole":         "clusterroles",
		"ClusterRoleBinding":  "clusterrolebindings",
		"NetworkPolicy":       "networkpolicies",
		"Secret":              "secrets",
		"Route":               "routes",
		"PersistentVolumeClaim": "persistentvolumeclaims",
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
					"app.kubernetes.io/managed-by": "hypershell",
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
					"app.kubernetes.io/managed-by": "hypershell",
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
					"app.kubernetes.io/managed-by": "hypershell",
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
