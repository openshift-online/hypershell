package gateway

import (
	"context"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stesting "k8s.io/client-go/testing"

	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

// routeResourceListKinds registers a list kind for every GVR RouteResourcesAbsent
// probes through the dynamic client, so the fake can resolve each Get.
func routeResourceListKinds() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "grpcroutes"}:         "GRPCRouteList",
		{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "backendtlspolicies"}: "BackendTLSPolicyList",
		{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "httproutes"}:         "HTTPRouteList",
		{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}:            "NetworkPolicyList",
		{Group: "apps", Version: "v1", Resource: "deployments"}:                             "DeploymentList",
	}
}

// TestRouteResourcesAbsent is the health loop's convergence backstop: teardown
// trusts its completion marker only while these probes confirm every route- and
// console-owned resource is actually gone, so a stale provisioning pass that
// recreates one after teardown cannot hide behind cleared address fields.
func TestRouteResourcesAbsent(t *testing.T) {
	const ns = "openshell-abc"

	t.Run("all absent returns true", func(t *testing.T) {
		dc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), routeResourceListKinds())
		cs := k8sfake.NewSimpleClientset()
		absent, err := RouteResourcesAbsent(context.Background(), dc, cs, ns, nil, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !absent {
			t.Fatal("want absent=true when no owned resources exist")
		}
	})

	t.Run("a resurrected dynamic resource returns false", func(t *testing.T) {
		dc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), routeResourceListKinds(),
			labeledResource("gateway.networking.k8s.io/v1", "GRPCRoute", ns, "openshell-gateway", true),
		)
		cs := k8sfake.NewSimpleClientset()
		absent, err := RouteResourcesAbsent(context.Background(), dc, cs, ns, nil, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if absent {
			t.Fatal("want absent=false when the GRPCRoute reappeared")
		}
	})

	t.Run("a resurrected typed resource returns false", func(t *testing.T) {
		dc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), routeResourceListKinds())
		cs := k8sfake.NewSimpleClientset(&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "openshell-backend-ca"},
		})
		absent, err := RouteResourcesAbsent(context.Background(), dc, cs, ns, nil, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if absent {
			t.Fatal("want absent=false when the backend-CA ConfigMap reappeared")
		}
	})

	t.Run("an unobservable probe returns an error, never false-absent", func(t *testing.T) {
		dc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), routeResourceListKinds())
		cs := k8sfake.NewSimpleClientset()
		cs.PrependReactor("get", "configmaps", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, fmt.Errorf("apiserver unavailable")
		})
		absent, err := RouteResourcesAbsent(context.Background(), dc, cs, ns, nil, "")
		if err == nil {
			t.Fatal("want an error when a probe cannot observe the resource")
		}
		if absent {
			t.Fatal("unknown state must never be reported as absent")
		}
	})

	// The Keycloak console client is a realm object, not a namespaced one: a stale
	// provisioning pass that recreated only the client would slip past every
	// Kubernetes probe above, so its residual existence must still block absence.
	t.Run("a residual keycloak console client returns false", func(t *testing.T) {
		dc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), routeResourceListKinds())
		cs := k8sfake.NewSimpleClientset()
		checker := &fakeConsoleClientChecker{exists: true}
		absent, err := RouteResourcesAbsent(context.Background(), dc, cs, ns, checker, "gw-1-console")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if absent {
			t.Fatal("want absent=false when the Keycloak console client still exists")
		}
	})

	// An unreachable Keycloak realm must surface as an error, never as absence:
	// teardown must re-run, not settle on an unknown external state.
	t.Run("an unobservable keycloak probe returns an error, never false-absent", func(t *testing.T) {
		dc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), routeResourceListKinds())
		cs := k8sfake.NewSimpleClientset()
		checker := &fakeConsoleClientChecker{err: fmt.Errorf("keycloak unreachable")}
		absent, err := RouteResourcesAbsent(context.Background(), dc, cs, ns, checker, "gw-1-console")
		if err == nil {
			t.Fatal("want an error when the Keycloak client cannot be observed")
		}
		if absent {
			t.Fatal("unknown external state must never be reported as absent")
		}
	})

	// With every Kubernetes resource gone and the console client also deleted,
	// teardown is genuinely settled -- including the external realm.
	t.Run("all absent including keycloak returns true", func(t *testing.T) {
		dc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), routeResourceListKinds())
		cs := k8sfake.NewSimpleClientset()
		checker := &fakeConsoleClientChecker{exists: false}
		absent, err := RouteResourcesAbsent(context.Background(), dc, cs, ns, checker, "gw-1-console")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !absent {
			t.Fatal("want absent=true when no Kubernetes resources and no console client remain")
		}
	})
}

// fakeConsoleClientChecker is a stub ConsoleClientChecker for driving the
// Keycloak console-client probe in RouteResourcesAbsent.
type fakeConsoleClientChecker struct {
	exists bool
	err    error
}

func (f *fakeConsoleClientChecker) ConsoleClientExists(context.Context, string) (bool, error) {
	return f.exists, f.err
}

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

func TestCopyDeploymentDatabaseCredentialsConvergesExistingSecret(t *testing.T) {
	const (
		sourceNamespace = "database-ns"
		tenantNamespace = "gateway-ns"
	)
	client := k8sfake.NewSimpleClientset(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "openshell-db-credentials", Namespace: sourceNamespace},
			Data: map[string][]byte{
				"dbname":   []byte("openshell"),
				"user":     []byte("openshell"),
				"password": []byte("new-password"),
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "openshell-gateway-db-credentials", Namespace: tenantNamespace},
			Data: map[string][]byte{
				"host":     []byte("stale-host"),
				"password": []byte("stale-password"),
			},
		},
	)

	if err := copyDeploymentDatabaseCredentials(context.Background(), client, sourceNamespace, tenantNamespace); err != nil {
		t.Fatalf("copyDeploymentDatabaseCredentials: %v", err)
	}

	got, err := client.CoreV1().Secrets(tenantNamespace).Get(context.Background(), "openshell-gateway-db-credentials", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get gateway credentials: %v", err)
	}
	if string(got.Data["host"]) != "openshell-gateway-db.database-ns.svc.cluster.local" {
		t.Fatalf("host = %q, want converged database service DNS", got.Data["host"])
	}
	if string(got.Data["password"]) != "new-password" {
		t.Fatalf("password = %q, want source password", got.Data["password"])
	}
	if got.Labels["hypershell.redhat.io/managed"] != "true" {
		t.Fatalf("managed label = %q, want true", got.Labels["hypershell.redhat.io/managed"])
	}
}

func TestCopyDeploymentDatabaseCredentialsRejectsIncompleteSource(t *testing.T) {
	client := k8sfake.NewSimpleClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "openshell-db-credentials", Namespace: "database-ns"},
		Data:       map[string][]byte{"user": []byte("openshell")},
	})

	err := copyDeploymentDatabaseCredentials(context.Background(), client, "database-ns", "gateway-ns")
	if err == nil {
		t.Fatal("copyDeploymentDatabaseCredentials = nil, want missing-key error")
	}
}
