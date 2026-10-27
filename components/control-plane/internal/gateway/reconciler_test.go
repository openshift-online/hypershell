package gateway

import (
	"context"
	"fmt"
	"reflect"
	"strings"
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
		{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gateways"}:           "GatewayList",
		{Group: "cert-manager.io", Version: "v1", Resource: "certificates"}:                 "CertificateList",
		{Group: "route.openshift.io", Version: "v1", Resource: "routes"}:                    "RouteList",
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
		absent, err := RouteResourcesAbsent(context.Background(), dc, cs, ns, IngressModeGatewayAPI, nil, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !absent {
			t.Fatal("want absent=true when no owned resources exist")
		}
	})

	t.Run("all absent in route mode returns true", func(t *testing.T) {
		dc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), routeResourceListKinds())
		cs := k8sfake.NewSimpleClientset()
		absent, err := RouteResourcesAbsent(context.Background(), dc, cs, ns, IngressModeRoute, nil, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !absent {
			t.Fatal("want absent=true when no Route-mode resources exist")
		}
	})

	t.Run("a resurrected dynamic resource returns false", func(t *testing.T) {
		dc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), routeResourceListKinds(),
			labeledResource("gateway.networking.k8s.io/v1", "GRPCRoute", ns, "openshell-gateway", true),
		)
		cs := k8sfake.NewSimpleClientset()
		absent, err := RouteResourcesAbsent(context.Background(), dc, cs, ns, IngressModeGatewayAPI, nil, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if absent {
			t.Fatal("want absent=false when the GRPCRoute reappeared")
		}
	})

	t.Run("route mode probes the gateway Route", func(t *testing.T) {
		dc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), routeResourceListKinds(),
			labeledResource("route.openshift.io/v1", "Route", ns, "openshell-gateway", true),
		)
		cs := k8sfake.NewSimpleClientset()
		absent, err := RouteResourcesAbsent(context.Background(), dc, cs, ns, IngressModeRoute, nil, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if absent {
			t.Fatal("want absent=false when the gateway Route reappeared")
		}
	})

	for _, tc := range []struct {
		name       string
		mode       string
		apiVersion string
		kind       string
	}{
		{
			name:       "gateway api mode probes an inactive console Route",
			mode:       IngressModeGatewayAPI,
			apiVersion: "route.openshift.io/v1",
			kind:       "Route",
		},
		{
			name:       "route mode probes an inactive console HTTPRoute",
			mode:       IngressModeRoute,
			apiVersion: "gateway.networking.k8s.io/v1",
			kind:       "HTTPRoute",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), routeResourceListKinds(),
				labeledResource(tc.apiVersion, tc.kind, ns, consoleName, true),
			)
			cs := k8sfake.NewSimpleClientset()
			absent, err := RouteResourcesAbsent(context.Background(), dc, cs, ns, tc.mode, nil, "")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if absent {
				t.Fatalf("want absent=false when the inactive console %s reappeared", tc.kind)
			}
		})
	}

	// Dedicated-certificate resources are probed unconditionally in Gateway API
	// mode: they exist only for opted-in gateways, so a residual one after
	// teardown means cleanup has not converged and must block the settled state.
	// The objects are seeded through the exact GVRs the probes use because the
	// fake client's constructor pluralizes "Gateway" naively ("gatewaies").
	for _, tc := range []struct {
		name    string
		gvr     schema.GroupVersionResource
		kind    string
		objName string
	}{
		{
			name:    "a residual external Certificate returns false",
			gvr:     dedicatedCertGVR,
			kind:    "Certificate",
			objName: "openshell-gateway-external",
		},
		{
			name:    "a residual per-tenant Gateway returns false",
			gvr:     dedicatedGatewayGVR,
			kind:    "Gateway",
			objName: "gw-openshell-abc",
		},
		{
			name:    "a residual sandbox HTTPRoute returns false",
			gvr:     dedicatedHTTPRouteGVR,
			kind:    "HTTPRoute",
			objName: "openshell-gateway-sandbox",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), routeResourceListKinds())
			seedDynamicResource(t, dc, tc.gvr, tc.kind, ns, tc.objName)
			cs := k8sfake.NewSimpleClientset()
			absent, err := RouteResourcesAbsent(context.Background(), dc, cs, ns, IngressModeGatewayAPI, nil, "")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if absent {
				t.Fatalf("want absent=false when the %s %s reappeared", tc.kind, tc.objName)
			}
		})
	}

	t.Run("a resurrected typed resource returns false", func(t *testing.T) {
		dc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), routeResourceListKinds())
		cs := k8sfake.NewSimpleClientset(&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "openshell-gateway-backend-ca"},
		})
		absent, err := RouteResourcesAbsent(context.Background(), dc, cs, ns, IngressModeGatewayAPI, nil, "")
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
		absent, err := RouteResourcesAbsent(context.Background(), dc, cs, ns, IngressModeGatewayAPI, nil, "")
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
		absent, err := RouteResourcesAbsent(context.Background(), dc, cs, ns, IngressModeGatewayAPI, checker, "gw-1-console")
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
		absent, err := RouteResourcesAbsent(context.Background(), dc, cs, ns, IngressModeGatewayAPI, checker, "gw-1-console")
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
		absent, err := RouteResourcesAbsent(context.Background(), dc, cs, ns, IngressModeGatewayAPI, checker, "gw-1-console")
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

// seedDynamicResource creates a labeled namespaced object through the exact
// GVR the production code addresses it by. The fake dynamic client's
// constructor pluralizes kinds naively ("Gateway" lands under "gatewaies"),
// so seeding through the constructor would make a Get through the real GVR
// miss and let an absence assertion pass vacuously.
func seedDynamicResource(t *testing.T, dc *dynamicfake.FakeDynamicClient, gvr schema.GroupVersionResource, kind, namespace, name string) {
	t.Helper()
	obj := labeledResource(gvr.Group+"/"+gvr.Version, kind, namespace, name, true)
	if _, err := dc.Resource(gvr).Namespace(namespace).Create(context.Background(), obj, metav1.CreateOptions{}); err != nil {
		t.Fatalf("seed %s %s/%s: %v", kind, namespace, name, err)
	}
}

// TestDeleteLabeledNamespaceResources verifies the surviving-namespace fallback
// reclaims only this gateway's labeled resources and leaves co-tenant workloads
// (and the namespace itself) untouched - the same no-collateral guarantee that
// keeps GC from reaping a shared namespace.
// Verifies: GTW-DEL-02.01
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

// TestReconcileRouteResourcesDoesNotCreateRoute verifies that
// reconcileRouteResources does NOT create a Route (the Helm chart owns the
// Route via openshiftRoute.enabled).
func TestReconcileRouteResourcesDoesNotCreateRoute(t *testing.T) {
	routesGVR := schema.GroupVersionResource{Group: "route.openshift.io", Version: "v1", Resource: "routes"}
	nsConfig := NamespaceConfig{
		Name: "openshell-test",
		Gateway: GatewayConfig{
			Route: RouteConfig{Enabled: true, Host: "gw-openshell-test.apps.example.com"},
		},
	}

	dc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), routeResourceListKinds())

	if err := reconcileRouteResources(context.Background(), dc, nsConfig, ReconcileOpts{}); err != nil {
		t.Fatalf("reconcileRouteResources: %v", err)
	}

	if _, err := dc.Resource(routesGVR).Namespace(nsConfig.Name).Get(context.Background(), "openshell-gateway", metav1.GetOptions{}); !k8serrors.IsNotFound(err) {
		t.Errorf("reconciler must not create Route (chart owns it), get err = %v", err)
	}
}

// --- Dedicated per-gateway certificate (route.certificate) ---

// GVRs of the resources only a dedicated-certificate gateway creates.
var (
	dedicatedCertGVR      = schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"}
	dedicatedGatewayGVR   = schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gateways"}
	dedicatedHTTPRouteGVR = schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "httproutes"}
	tenantGRPCRouteGVR    = schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "grpcroutes"}
	tenantBTLSPolicyGVR   = schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "backendtlspolicies"}
)

// dedicatedCertificateListKinds registers a list kind for every GVR the
// dedicated-certificate reconcile path touches through the dynamic client.
func dedicatedCertificateListKinds() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		tenantGRPCRouteGVR:    "GRPCRouteList",
		tenantBTLSPolicyGVR:   "BackendTLSPolicyList",
		dedicatedGatewayGVR:   "GatewayList",
		dedicatedHTTPRouteGVR: "HTTPRouteList",
		dedicatedCertGVR:      "CertificateList",
		{Group: "apps", Version: "v1", Resource: "deployments"}: "DeploymentList",
	}
}

// mustGetDynamic fetches a namespaced resource through the dynamic client,
// failing the test when it is missing (so a "resource was created" assertion
// cannot pass vacuously on a NotFound).
func mustGetDynamic(t *testing.T, dc *dynamicfake.FakeDynamicClient, gvr schema.GroupVersionResource, namespace, name string) *unstructured.Unstructured {
	t.Helper()
	obj, err := dc.Resource(gvr).Namespace(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get %s %s/%s: %v", gvr.Resource, namespace, name, err)
	}
	return obj
}

// mustNotExistDynamic asserts a namespaced resource is absent.
func mustNotExistDynamic(t *testing.T, dc *dynamicfake.FakeDynamicClient, gvr schema.GroupVersionResource, namespace, name string) {
	t.Helper()
	_, err := dc.Resource(gvr).Namespace(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if !k8serrors.IsNotFound(err) {
		t.Errorf("%s %s/%s must not exist, get err = %v", gvr.Resource, namespace, name, err)
	}
}

// TestInjectIngressHostnames guards the pre-Helm SAN injection: the derived
// hostname must land in server_sans for every routed gateway, and a
// dedicated-certificate gateway must additionally get the wildcard so the
// gateway can derive its sandbox-service base domain (Gap C in
// docs/investigations/per-gateway-certificates.md).
func TestInjectIngressHostnames(t *testing.T) {
	const ns = "openshell-a1b2c3"
	const baseDomain = "hyp0-spoke0.infra.hypershell.app"
	const hostname = "gw-" + ns + "." + baseDomain

	t.Run("dedicated certificate injects exact and wildcard SANs", func(t *testing.T) {
		t.Setenv("GATEWAY_API_BASE_DOMAIN", baseDomain)
		nsConfig := &NamespaceConfig{
			Name: ns,
			Gateway: GatewayConfig{
				ServerDnsNames: []string{"openshell-gateway." + ns + ".svc.cluster.local"},
				Route:          RouteConfig{Enabled: true, Certificate: &RouteCertificateConfig{}},
			},
		}
		injectIngressHostnames(nsConfig)
		want := []string{
			"openshell-gateway." + ns + ".svc.cluster.local",
			hostname,
			"*." + hostname,
		}
		if !reflect.DeepEqual(nsConfig.Gateway.ServerDnsNames, want) {
			t.Errorf("ServerDnsNames = %v, want %v", nsConfig.Gateway.ServerDnsNames, want)
		}
		if nsConfig.Gateway.Route.Host != hostname {
			t.Errorf("Route.Host = %q, want the derived %q", nsConfig.Gateway.Route.Host, hostname)
		}
	})

	t.Run("shared path injects the exact hostname only", func(t *testing.T) {
		t.Setenv("GATEWAY_API_BASE_DOMAIN", baseDomain)
		nsConfig := &NamespaceConfig{
			Name:    ns,
			Gateway: GatewayConfig{Route: RouteConfig{Enabled: true}},
		}
		injectIngressHostnames(nsConfig)
		want := []string{hostname}
		if !reflect.DeepEqual(nsConfig.Gateway.ServerDnsNames, want) {
			t.Errorf("ServerDnsNames = %v, want %v (no wildcard on the shared path)", nsConfig.Gateway.ServerDnsNames, want)
		}
	})

	t.Run("derivation failure leaves the config unchanged", func(t *testing.T) {
		t.Setenv("GATEWAY_API_BASE_DOMAIN", "")
		nsConfig := &NamespaceConfig{
			Name:    ns,
			Gateway: GatewayConfig{ServerDnsNames: []string{"existing.example.com"}, Route: RouteConfig{Enabled: true}},
		}
		injectIngressHostnames(nsConfig)
		want := []string{"existing.example.com"}
		if !reflect.DeepEqual(nsConfig.Gateway.ServerDnsNames, want) {
			t.Errorf("ServerDnsNames = %v, want unchanged %v", nsConfig.Gateway.ServerDnsNames, want)
		}
		if nsConfig.Gateway.Route.Host != "" {
			t.Errorf("Route.Host = %q, want empty", nsConfig.Gateway.Route.Host)
		}
	})
}

// TestReconcileGatewayAPIResourcesDedicatedCertificate verifies the full
// dedicated-certificate resource set an opted-in gateway gets: the cert-manager
// Certificate with both DNS names, the per-tenant Gateway with two listeners and
// the external-dns annotation, the GRPCRoute re-parented onto the per-tenant
// Gateway, and the wildcard sandbox HTTPRoute.
func TestReconcileGatewayAPIResourcesDedicatedCertificate(t *testing.T) {
	const ns = "openshell-a1b2c3"
	const baseDomain = "hyp0-spoke0.infra.hypershell.app"
	const hostname = "gw-" + ns + "." + baseDomain
	const wildcard = "*." + hostname

	reconcileOptedIn := func(t *testing.T) (*dynamicfake.FakeDynamicClient, *k8sfake.Clientset) {
		t.Helper()
		t.Setenv("GATEWAY_API_BASE_DOMAIN", baseDomain)
		t.Setenv("GATEWAY_EXTERNAL_ISSUER", "letsencrypt-dns01")
		// The shared Gateway must NOT be required on the dedicated-certificate path.
		t.Setenv("GATEWAY_API_GATEWAY_NAME", "")

		dc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), dedicatedCertificateListKinds())
		cs := k8sfake.NewSimpleClientset(
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "openshell-server-tls"},
				Data:       map[string][]byte{"ca.crt": []byte("CA-PEM")},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "openshell-gateway-external-tls"},
				Data:       map[string][]byte{"tls.crt": []byte("EXTERNAL-CERT")},
			},
		)
		nsConfig := NamespaceConfig{
			Name: ns,
			Gateway: GatewayConfig{Route: RouteConfig{
				Enabled:     true,
				Host:        hostname, // as injectIngressHostnames fills in before Helm
				Certificate: &RouteCertificateConfig{},
			}},
		}
		if err := reconcileGatewayAPIRouteResources(context.Background(), dc, cs, nsConfig, ReconcileOpts{HasGatewayAPI: true}); err != nil {
			t.Fatalf("reconcileGatewayAPIRouteResources: %v", err)
		}
		return dc, cs
	}

	t.Run("certificate carries both DNS names and the cluster issuer", func(t *testing.T) {
		dc, _ := reconcileOptedIn(t)
		cert := mustGetDynamic(t, dc, dedicatedCertGVR, ns, externalCertificateName)
		if name := cert.GetName(); name != externalCertificateName {
			t.Errorf("Certificate name = %q, want %q", name, externalCertificateName)
		}
		if v, _, _ := unstructured.NestedString(cert.Object, "spec", "secretName"); v != externalCertificateSecretName {
			t.Errorf("spec.secretName = %q, want %q", v, externalCertificateSecretName)
		}
		dnsNames, _, _ := unstructured.NestedStringSlice(cert.Object, "spec", "dnsNames")
		if !reflect.DeepEqual(dnsNames, []string{hostname, wildcard}) {
			t.Errorf("spec.dnsNames = %v, want [%s %s]", dnsNames, hostname, wildcard)
		}
		if v, _, _ := unstructured.NestedString(cert.Object, "spec", "rotationPolicy"); v != "Always" {
			t.Errorf("spec.rotationPolicy = %q, want Always", v)
		}
		if v, _, _ := unstructured.NestedString(cert.Object, "spec", "issuerRef", "name"); v != "letsencrypt-dns01" {
			t.Errorf("spec.issuerRef.name = %q, want letsencrypt-dns01", v)
		}
		if v, _, _ := unstructured.NestedString(cert.Object, "spec", "issuerRef", "kind"); v != "ClusterIssuer" {
			t.Errorf("spec.issuerRef.kind = %q, want ClusterIssuer (default)", v)
		}
		if v, _, _ := unstructured.NestedString(cert.Object, "spec", "issuerRef", "group"); v != "cert-manager.io" {
			t.Errorf("spec.issuerRef.group = %q, want cert-manager.io", v)
		}
	})

	t.Run("per-tenant gateway has two TLS listeners and publishes both DNS records", func(t *testing.T) {
		dc, _ := reconcileOptedIn(t)
		gw := mustGetDynamic(t, dc, dedicatedGatewayGVR, ns, perTenantGatewayName(ns))
		if v, _, _ := unstructured.NestedString(gw.Object, "spec", "gatewayClassName"); v != "openshift-default" {
			t.Errorf("spec.gatewayClassName = %q, want openshift-default (default)", v)
		}
		if v, _, _ := unstructured.NestedString(gw.Object, "spec", "infrastructure", "annotations", "external-dns.kubernetes.io/hostname"); v != hostname+","+wildcard {
			t.Errorf("external-dns annotation = %q, want %q", v, hostname+","+wildcard)
		}
		listeners, _, _ := unstructured.NestedSlice(gw.Object, "spec", "listeners")
		if len(listeners) != 2 {
			t.Fatalf("len(listeners) = %d, want 2", len(listeners))
		}
		assertPerTenantGatewayListener(t, listeners, "grpc", hostname, "GRPCRoute")
		assertPerTenantGatewayListener(t, listeners, "sandbox", wildcard, "HTTPRoute")
	})

	t.Run("grpcroute re-parents onto the per-tenant gateway", func(t *testing.T) {
		dc, _ := reconcileOptedIn(t)
		grpcRoute := mustGetDynamic(t, dc, tenantGRPCRouteGVR, ns, "openshell-gateway")
		parents, _, _ := unstructured.NestedSlice(grpcRoute.Object, "spec", "parentRefs")
		if len(parents) != 1 {
			t.Fatalf("len(parentRefs) = %d, want 1", len(parents))
		}
		parent, ok := parents[0].(map[string]interface{})
		if !ok {
			t.Fatalf("parentRefs[0] is not a map: %T", parents[0])
		}
		if parent["name"] != perTenantGatewayName(ns) {
			t.Errorf("parentRef.name = %v, want %q", parent["name"], perTenantGatewayName(ns))
		}
		if parent["namespace"] != ns {
			t.Errorf("parentRef.namespace = %v, want %q (same namespace, no ReferenceGrant)", parent["namespace"], ns)
		}
		if parent["sectionName"] != "grpc" {
			t.Errorf("parentRef.sectionName = %v, want grpc", parent["sectionName"])
		}
		hostnames, _, _ := unstructured.NestedStringSlice(grpcRoute.Object, "spec", "hostnames")
		if !reflect.DeepEqual(hostnames, []string{hostname}) {
			t.Errorf("spec.hostnames = %v, want [%s]", hostnames, hostname)
		}
	})

	t.Run("sandbox httproute wildcard-hosts onto the sandbox listener", func(t *testing.T) {
		dc, _ := reconcileOptedIn(t)
		httpRoute := mustGetDynamic(t, dc, dedicatedHTTPRouteGVR, ns, sandboxHTTPRouteName)
		hostnames, _, _ := unstructured.NestedStringSlice(httpRoute.Object, "spec", "hostnames")
		if !reflect.DeepEqual(hostnames, []string{wildcard}) {
			t.Errorf("spec.hostnames = %v, want [%s]", hostnames, wildcard)
		}
		parents, _, _ := unstructured.NestedSlice(httpRoute.Object, "spec", "parentRefs")
		if len(parents) != 1 {
			t.Fatalf("len(parentRefs) = %d, want 1", len(parents))
		}
		parent, ok := parents[0].(map[string]interface{})
		if !ok {
			t.Fatalf("parentRefs[0] is not a map: %T", parents[0])
		}
		if parent["name"] != perTenantGatewayName(ns) {
			t.Errorf("parentRef.name = %v, want %q", parent["name"], perTenantGatewayName(ns))
		}
		if parent["sectionName"] != "sandbox" {
			t.Errorf("parentRef.sectionName = %v, want sandbox", parent["sectionName"])
		}
		backends, _, _ := unstructured.NestedSlice(httpRoute.Object, "spec", "rules")
		rule, ok := backends[0].(map[string]interface{})
		if !ok {
			t.Fatalf("rules[0] is not a map: %T", backends[0])
		}
		backendRefs, _, _ := unstructured.NestedSlice(rule, "backendRefs")
		if len(backendRefs) != 1 {
			t.Fatalf("len(backendRefs) = %d, want 1", len(backendRefs))
		}
		backend, ok := backendRefs[0].(map[string]interface{})
		if !ok {
			t.Fatalf("backendRefs[0] is not a map: %T", backendRefs[0])
		}
		port, _, _ := unstructured.NestedInt64(backend, "port")
		if backend["name"] != "openshell-gateway" || port != 8080 {
			t.Errorf("backendRef = %v, want openshell-gateway:8080", backend)
		}
	})

	t.Run("issuer kind and gateway class honor env overrides", func(t *testing.T) {
		t.Setenv("GATEWAY_EXTERNAL_ISSUER_KIND", "Issuer")
		t.Setenv("GATEWAY_API_GATEWAY_CLASS", "istio")
		dc, _ := reconcileOptedIn(t)
		cert := mustGetDynamic(t, dc, dedicatedCertGVR, ns, externalCertificateName)
		if v, _, _ := unstructured.NestedString(cert.Object, "spec", "issuerRef", "kind"); v != "Issuer" {
			t.Errorf("spec.issuerRef.kind = %q, want Issuer (env override)", v)
		}
		gw := mustGetDynamic(t, dc, dedicatedGatewayGVR, ns, perTenantGatewayName(ns))
		if v, _, _ := unstructured.NestedString(gw.Object, "spec", "gatewayClassName"); v != "istio" {
			t.Errorf("spec.gatewayClassName = %q, want istio (env override)", v)
		}
	})
}

// assertPerTenantGatewayListener checks one listener of the per-tenant Gateway:
// port 443, HTTPS, TLS termination on the external secret, the expected
// hostname, the expected allowed route kind, and Same-namespace routing.
func assertPerTenantGatewayListener(t *testing.T, listeners []interface{}, name, hostname, routeKind string) {
	t.Helper()
	for _, l := range listeners {
		listener, ok := l.(map[string]interface{})
		if !ok {
			continue
		}
		if n, _ := listener["name"].(string); n != name {
			continue
		}
		if h, _ := listener["hostname"].(string); h != hostname {
			t.Errorf("listener %q hostname = %q, want %q", name, h, hostname)
		}
		if p, _, _ := unstructured.NestedInt64(listener, "port"); p != 443 {
			t.Errorf("listener %q port = %d, want 443", name, p)
		}
		if proto, _ := listener["protocol"].(string); proto != "HTTPS" {
			t.Errorf("listener %q protocol = %q, want HTTPS", name, proto)
		}
		if mode, _, _ := unstructured.NestedString(listener, "tls", "mode"); mode != "Terminate" {
			t.Errorf("listener %q tls.mode = %q, want Terminate", name, mode)
		}
		certRefs, _, _ := unstructured.NestedSlice(listener, "tls", "certificateRefs")
		if len(certRefs) != 1 {
			t.Fatalf("listener %q has %d certificateRefs, want 1", name, len(certRefs))
		}
		certRef, ok := certRefs[0].(map[string]interface{})
		if !ok {
			t.Fatalf("listener %q certificateRefs[0] is not a map: %T", name, certRefs[0])
		}
		if certRef["group"] != "" || certRef["kind"] != "Secret" || certRef["name"] != externalCertificateSecretName {
			t.Errorf("listener %q certificateRef = %v, want Secret %s", name, certRef, externalCertificateSecretName)
		}
		kindsSlice, _, _ := unstructured.NestedSlice(listener, "allowedRoutes", "kinds")
		if len(kindsSlice) != 1 {
			t.Fatalf("listener %q has %d allowedRoutes kinds, want 1", name, len(kindsSlice))
		}
		kindRef, ok := kindsSlice[0].(map[string]interface{})
		if !ok {
			t.Fatalf("listener %q allowedRoutes kinds[0] is not a map: %T", name, kindsSlice[0])
		}
		if kindRef["kind"] != routeKind || kindRef["group"] != "gateway.networking.k8s.io" {
			t.Errorf("listener %q allowedRoutes kind = %v, want group gateway.networking.k8s.io kind %s", name, kindRef, routeKind)
		}
		if from, _, _ := unstructured.NestedString(listener, "allowedRoutes", "namespaces", "from"); from != "Same" {
			t.Errorf("listener %q allowedRoutes.namespaces.from = %q, want Same", name, from)
		}
		return
	}
	t.Fatalf("listener %q not found", name)
}

// TestReconcileGatewayAPIResourcesDedicatedCertificateRequiresIssuer verifies
// the controller env-var gate: an opted-in gateway fails provisioning with an
// error naming GATEWAY_EXTERNAL_ISSUER when the controller has none configured,
// and nothing is created.
func TestReconcileGatewayAPIResourcesDedicatedCertificateRequiresIssuer(t *testing.T) {
	const ns = "openshell-a1b2c3"
	t.Setenv("GATEWAY_API_BASE_DOMAIN", "hyp0-spoke0.infra.hypershell.app")
	t.Setenv("GATEWAY_EXTERNAL_ISSUER", "")

	dc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), dedicatedCertificateListKinds())
	cs := k8sfake.NewSimpleClientset()
	nsConfig := NamespaceConfig{
		Name:    ns,
		Gateway: GatewayConfig{Route: RouteConfig{Enabled: true, Certificate: &RouteCertificateConfig{}}},
	}

	err := reconcileGatewayAPIRouteResources(context.Background(), dc, cs, nsConfig, ReconcileOpts{HasGatewayAPI: true})
	if err == nil {
		t.Fatal("expected an error when GATEWAY_EXTERNAL_ISSUER is unset")
	}
	if !strings.Contains(err.Error(), "GATEWAY_EXTERNAL_ISSUER") {
		t.Errorf("error does not name the missing env var: %v", err)
	}

	mustNotExistDynamic(t, dc, dedicatedCertGVR, ns, externalCertificateName)
	mustNotExistDynamic(t, dc, dedicatedGatewayGVR, ns, perTenantGatewayName(ns))
	mustNotExistDynamic(t, dc, tenantGRPCRouteGVR, ns, "openshell-gateway")
	mustNotExistDynamic(t, dc, dedicatedHTTPRouteGVR, ns, sandboxHTTPRouteName)
}

// TestReconcileGatewayAPIResourcesSharedGatewayUnchanged verifies a
// non-opted-in gateway keeps byte-for-byte today's behavior: the GRPCRoute
// parents onto the shared Gateway and no dedicated-certificate resource is
// created.
func TestReconcileGatewayAPIResourcesSharedGatewayUnchanged(t *testing.T) {
	const ns = "openshell-a1b2c3"
	t.Setenv("GATEWAY_API_BASE_DOMAIN", "hyp0-spoke0.infra.hypershell.app")
	t.Setenv("GATEWAY_API_GATEWAY_NAME", "openshell-grpc-gateway")
	t.Setenv("GATEWAY_API_GATEWAY_NAMESPACE", "openshift-ingress")
	t.Setenv("GATEWAY_API_HTTP_LISTENER_NAME", "grpc-hyp0")

	dc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), dedicatedCertificateListKinds())
	cs := k8sfake.NewSimpleClientset(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "openshell-server-tls"},
			Data:       map[string][]byte{"ca.crt": []byte("CA-PEM")},
		},
	)
	nsConfig := NamespaceConfig{
		Name: ns,
		Gateway: GatewayConfig{Route: RouteConfig{
			Enabled: true,
			Host:    "gw-" + ns + ".hyp0-spoke0.infra.hypershell.app",
		}},
	}

	if err := reconcileGatewayAPIRouteResources(context.Background(), dc, cs, nsConfig, ReconcileOpts{HasGatewayAPI: true}); err != nil {
		t.Fatalf("reconcileGatewayAPIRouteResources: %v", err)
	}

	// The GRPCRoute still parents onto the shared Gateway.
	grpcRoute := mustGetDynamic(t, dc, tenantGRPCRouteGVR, ns, "openshell-gateway")
	parents, _, _ := unstructured.NestedSlice(grpcRoute.Object, "spec", "parentRefs")
	if len(parents) != 1 {
		t.Fatalf("len(parentRefs) = %d, want 1", len(parents))
	}
	parent, ok := parents[0].(map[string]interface{})
	if !ok {
		t.Fatalf("parentRefs[0] is not a map: %T", parents[0])
	}
	if parent["name"] != "openshell-grpc-gateway" || parent["namespace"] != "openshift-ingress" || parent["sectionName"] != "grpc-hyp0" {
		t.Errorf("parentRef = %v, want shared Gateway openshell-ingress/openshell-grpc-gateway section grpc-hyp0", parent)
	}

	// No dedicated-certificate resource is created.
	mustNotExistDynamic(t, dc, dedicatedCertGVR, ns, externalCertificateName)
	mustNotExistDynamic(t, dc, dedicatedGatewayGVR, ns, perTenantGatewayName(ns))
	mustNotExistDynamic(t, dc, dedicatedHTTPRouteGVR, ns, sandboxHTTPRouteName)
}

// TestDeleteGatewayAPIRouteResourcesRemovesDedicatedCertificateResources
// verifies teardown reclaims the per-tenant Gateway, external Certificate, and
// sandbox HTTPRoute (idempotently - a second pass on an empty namespace is a
// no-op), so a removed gateway leaves no live listener serving its hostname or
// wildcard.
func TestDeleteGatewayAPIRouteResourcesRemovesDedicatedCertificateResources(t *testing.T) {
	const ns = "openshell-abc"

	dc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), dedicatedCertificateListKinds())
	// Seed through the exact GVRs the deletes address (see seedDynamicResource).
	for _, seeded := range []struct {
		gvr  schema.GroupVersionResource
		kind string
		name string
	}{
		{tenantGRPCRouteGVR, "GRPCRoute", "openshell-gateway"},
		{tenantBTLSPolicyGVR, "BackendTLSPolicy", "openshell-gateway"},
		{dedicatedCertGVR, "Certificate", externalCertificateName},
		{dedicatedGatewayGVR, "Gateway", perTenantGatewayName(ns)},
		{dedicatedHTTPRouteGVR, "HTTPRoute", sandboxHTTPRouteName},
	} {
		seedDynamicResource(t, dc, seeded.gvr, seeded.kind, ns, seeded.name)
	}
	cs := k8sfake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: backendCAConfigMapName},
	})

	if err := deleteGatewayAPIRouteResources(context.Background(), dc, cs, ns); err != nil {
		t.Fatalf("deleteGatewayAPIRouteResources: %v", err)
	}

	for _, tc := range []struct {
		gvr  schema.GroupVersionResource
		name string
	}{
		{tenantGRPCRouteGVR, "openshell-gateway"},
		{tenantBTLSPolicyGVR, "openshell-gateway"},
		{dedicatedCertGVR, externalCertificateName},
		{dedicatedGatewayGVR, perTenantGatewayName(ns)},
		{dedicatedHTTPRouteGVR, sandboxHTTPRouteName},
	} {
		mustNotExistDynamic(t, dc, tc.gvr, ns, tc.name)
	}
	if _, err := cs.CoreV1().ConfigMaps(ns).Get(context.Background(), backendCAConfigMapName, metav1.GetOptions{}); !k8serrors.IsNotFound(err) {
		t.Errorf("backend CA ConfigMap still present after cleanup, err = %v", err)
	}

	// Idempotent: a second pass over the now-empty namespace succeeds.
	if err := deleteGatewayAPIRouteResources(context.Background(), dc, cs, ns); err != nil {
		t.Fatalf("deleteGatewayAPIRouteResources (second pass): %v", err)
	}
}
