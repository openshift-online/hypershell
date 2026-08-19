package gateway

import (
	"context"
	"testing"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

// labeledResource builds an unstructured namespaced object, optionally carrying
// the hypershell.redhat.io/managed label the gateway stamps on everything it
// creates.
func labeledResource(apiVersion, kind, namespace, name string, managed bool) *unstructured.Unstructured {
	meta := map[string]interface{}{
		"name":      name,
		"namespace": namespace,
	}
	if managed {
		meta["labels"] = map[string]interface{}{ManagedLabel: ManagedLabelValue}
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata":   meta,
	}}
}

// TestDeleteLabeledNamespaceResources verifies the surviving-namespace fallback
// reclaims only this gateway's labeled resources and leaves co-tenant workloads
// (and the namespace itself) untouched - the same no-collateral guarantee that
// keeps GC from reaping a shared namespace.
func TestDeleteLabeledNamespaceResources(t *testing.T) {
	const ns = "shared"

	depGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	stsGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}
	svcGVR := schema.GroupVersionResource{Version: "v1", Resource: "services"}
	secretGVR := schema.GroupVersionResource{Version: "v1", Resource: "secrets"}

	// The helper lists every GVR in its default set, so register a list kind for
	// each even though only a few are populated; an unregistered GVR would make
	// the fake client's List return an error.
	gvrToListKind := map[schema.GroupVersionResource]string{
		depGVR:                                  "DeploymentList",
		stsGVR:                                  "StatefulSetList",
		svcGVR:                                  "ServiceList",
		secretGVR:                               "SecretList",
		{Version: "v1", Resource: "configmaps"}: "ConfigMapList",
		{Version: "v1", Resource: "serviceaccounts"}:                                  "ServiceAccountList",
		{Version: "v1", Resource: "persistentvolumeclaims"}:                           "PersistentVolumeClaimList",
		{Group: "batch", Version: "v1", Resource: "jobs"}:                             "JobList",
		{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}:      "NetworkPolicyList",
		{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles"}:        "RoleList",
		{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings"}: "RoleBindingList",
	}

	dc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gvrToListKind,
		labeledResource("apps/v1", "Deployment", ns, "gw-deploy", true),
		labeledResource("apps/v1", "Deployment", ns, "tenant-deploy", false),
		labeledResource("apps/v1", "StatefulSet", ns, "gw-sts", true),
		labeledResource("v1", "Service", ns, "gw-svc", true),
		labeledResource("v1", "Secret", ns, "gw-secret", true),
	)

	// Default opts: the optional cert-manager / Gateway API GVRs are not swept, so
	// they need not be registered above.
	DeleteLabeledNamespaceResources(context.Background(), dc, ns, ReconcileOpts{})

	// Every labeled resource this gateway created is gone.
	labeled := []struct {
		gvr  schema.GroupVersionResource
		name string
	}{
		{depGVR, "gw-deploy"},
		{stsGVR, "gw-sts"},
		{svcGVR, "gw-svc"},
		{secretGVR, "gw-secret"},
	}
	for _, tc := range labeled {
		_, err := dc.Resource(tc.gvr).Namespace(ns).Get(context.Background(), tc.name, metav1.GetOptions{})
		if !k8serrors.IsNotFound(err) {
			t.Errorf("%s %s still present after cleanup, err = %v", tc.gvr.Resource, tc.name, err)
		}
	}

	// The co-tenant (unlabeled) deployment survives untouched.
	remaining, err := dc.Resource(depGVR).Namespace(ns).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list deployments: %v", err)
	}
	if len(remaining.Items) != 1 || remaining.Items[0].GetName() != "tenant-deploy" {
		var names []string
		for i := range remaining.Items {
			names = append(names, remaining.Items[i].GetName())
		}
		t.Errorf("deployments after cleanup = %v, want [tenant-deploy]", names)
	}
}
