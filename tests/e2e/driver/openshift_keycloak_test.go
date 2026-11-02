package driver

import (
	"context"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

// keycloakRoute builds an unstructured route.openshift.io/v1 Route with the given
// namespace, name, and spec.host, matching what the dynamic client returns.
func keycloakRoute(ns, name, host string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "route.openshift.io/v1",
		"kind":       "Route",
		"metadata":   map[string]any{"namespace": ns, "name": name},
		"spec":       map[string]any{"host": host},
	}}
}

func fakeRouteDriver(platformNS, route string, objs ...runtime.Object) *openshiftDriver {
	scheme := runtime.NewScheme()
	gvrToListKind := map[schema.GroupVersionResource]string{routeGVR: "RouteList"}
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, objs...)
	return &openshiftDriver{dyn: dyn, platformNS: platformNS, keycloakRoute: route}
}

func TestDiscoverKeycloakHostConventionalNamespace(t *testing.T) {
	d := fakeRouteDriver("hypershell", "keycloak",
		keycloakRoute("hypershell-keycloak", "keycloak", "sso.example.com"))

	host, err := d.discoverKeycloakHost(context.Background())
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if host != "sso.example.com" {
		t.Errorf("host = %q, want sso.example.com", host)
	}
	if d.keycloakNS != "hypershell-keycloak" {
		t.Errorf("resolved namespace = %q, want hypershell-keycloak", d.keycloakNS)
	}
}

// A GitOps layout that places Keycloak outside the "<platform>-keycloak"
// convention (e.g. keycloak-system) must still resolve, by cluster-wide name.
func TestDiscoverKeycloakHostClusterWideFallback(t *testing.T) {
	d := fakeRouteDriver("hypershell", "keycloak",
		keycloakRoute("keycloak-system", "keycloak", "sso.fleet.example.com"))

	host, err := d.discoverKeycloakHost(context.Background())
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if host != "sso.fleet.example.com" {
		t.Errorf("host = %q, want sso.fleet.example.com", host)
	}
	if d.keycloakNS != "keycloak-system" {
		t.Errorf("resolved namespace = %q, want keycloak-system", d.keycloakNS)
	}
}

// An explicit namespace override is authoritative and does not fall back.
func TestDiscoverKeycloakHostExplicitNamespaceOverride(t *testing.T) {
	d := fakeRouteDriver("hypershell", "keycloak",
		keycloakRoute("hypershell-keycloak", "keycloak", "convention.example.com"),
		keycloakRoute("keycloak-system", "keycloak", "override.example.com"))
	t.Setenv("E2E_OPENSHIFT_KEYCLOAK_NAMESPACE", "keycloak-system")

	host, err := d.discoverKeycloakHost(context.Background())
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if host != "override.example.com" {
		t.Errorf("host = %q, want override.example.com", host)
	}
}

// A wrong explicit override fails loudly rather than silently resolving elsewhere.
func TestDiscoverKeycloakHostExplicitNamespaceMissIsFatal(t *testing.T) {
	d := fakeRouteDriver("hypershell", "keycloak",
		keycloakRoute("keycloak-system", "keycloak", "sso.example.com"))
	t.Setenv("E2E_OPENSHIFT_KEYCLOAK_NAMESPACE", "wrong-ns")

	if _, err := d.discoverKeycloakHost(context.Background()); err == nil {
		t.Fatal("miss under explicit namespace override was not an error")
	}
}

// A custom route name (E2E_OPENSHIFT_KEYCLOAK_ROUTE) is honored by the search.
func TestDiscoverKeycloakHostCustomRouteName(t *testing.T) {
	d := fakeRouteDriver("hypershell", "sso",
		keycloakRoute("keycloak-system", "sso", "sso.example.com"))

	host, err := d.discoverKeycloakHost(context.Background())
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if host != "sso.example.com" {
		t.Errorf("host = %q, want sso.example.com", host)
	}
}

// Two Routes named "keycloak" in different namespaces is ambiguous: the caller
// must pin E2E_OPENSHIFT_KEYCLOAK_NAMESPACE rather than have one picked at random.
func TestDiscoverKeycloakHostAmbiguousIsError(t *testing.T) {
	d := fakeRouteDriver("hypershell", "keycloak",
		keycloakRoute("keycloak-system", "keycloak", "a.example.com"),
		keycloakRoute("another-ns", "keycloak", "b.example.com"))

	_, err := d.discoverKeycloakHost(context.Background())
	if err == nil {
		t.Fatal("ambiguous keycloak routes were not reported as an error")
	}
	for _, want := range []string{"keycloak-system", "another-ns"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name namespace %q", err, want)
		}
	}
}

// No Keycloak Route anywhere yields an actionable error naming the escape hatches.
func TestDiscoverKeycloakHostNotFound(t *testing.T) {
	d := fakeRouteDriver("hypershell", "keycloak")

	_, err := d.discoverKeycloakHost(context.Background())
	if err == nil {
		t.Fatal("missing keycloak route was not reported as an error")
	}
	for _, want := range []string{"E2E_OPENSHIFT_KEYCLOAK_NAMESPACE", "E2E_OIDC_ISSUER"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}
