package gateway

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
)

// consoleContainerByName returns the named container map from a console
// Deployment built by buildConsoleDeployment.
func consoleContainerByName(t *testing.T, dep *unstructured.Unstructured, name string) map[string]interface{} {
	t.Helper()
	containers, found, err := unstructured.NestedSlice(dep.Object, "spec", "template", "spec", "containers")
	if err != nil || !found {
		t.Fatalf("containers not found: found=%v err=%v", found, err)
	}
	for _, c := range containers {
		m, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if n, _, _ := unstructured.NestedString(m, "name"); n == name {
			return m
		}
	}
	t.Fatalf("container %q not found", name)
	return nil
}

func envValue(container map[string]interface{}, name string) (string, bool) {
	env, _, _ := unstructured.NestedSlice(container, "env")
	for _, e := range env {
		m, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		if n, _, _ := unstructured.NestedString(m, "name"); n == name {
			v, _, _ := unstructured.NestedString(m, "value")
			return v, true
		}
	}
	return "", false
}

func volumeNames(t *testing.T, dep *unstructured.Unstructured) map[string]bool {
	t.Helper()
	vols, _, _ := unstructured.NestedSlice(dep.Object, "spec", "template", "spec", "volumes")
	out := map[string]bool{}
	for _, v := range vols {
		m, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		if n, _, _ := unstructured.NestedString(m, "name"); n != "" {
			out[n] = true
		}
	}
	return out
}

func volumeMountNames(container map[string]interface{}) map[string]bool {
	mounts, _, _ := unstructured.NestedSlice(container, "volumeMounts")
	out := map[string]bool{}
	for _, m := range mounts {
		mm, ok := m.(map[string]interface{})
		if !ok {
			continue
		}
		if n, _, _ := unstructured.NestedString(mm, "name"); n != "" {
			out[n] = true
		}
	}
	return out
}

// When the issuer is served with a privately-signed certificate (trustedCA), the
// oauth2-proxy sidecar must mount the CA bundle and be pointed at it for OIDC
// discovery -- otherwise discovery fails with an x509 unknown-authority error.
func TestBuildConsoleDeployment_TrustedCAWiresOAuth2ProxyCA(t *testing.T) {
	dep := buildConsoleDeployment("ns", "dash:latest", "proxy:latest",
		"https://issuer.example/realms/r", "gw-1-console", "https://console.example/oauth2/callback", true)

	proxy := consoleContainerByName(t, dep, "oauth2-proxy")

	caFile, ok := envValue(proxy, "OAUTH2_PROXY_PROVIDER_CA_FILES")
	if !ok {
		t.Fatal("expected OAUTH2_PROXY_PROVIDER_CA_FILES to be set when trustedCA is true")
	}
	if caFile != consoleTrustedCAMountPath {
		t.Errorf("OAUTH2_PROXY_PROVIDER_CA_FILES = %q, want %q", caFile, consoleTrustedCAMountPath)
	}
	if !volumeNames(t, dep)["oidc-trusted-ca"] {
		t.Error("expected oidc-trusted-ca volume on the pod spec")
	}
	if !volumeMountNames(proxy)["oidc-trusted-ca"] {
		t.Error("expected oidc-trusted-ca volumeMount on the oauth2-proxy container")
	}
}

// The dashboard dials the gateway admin API over mutual TLS. It must use the
// grpcs:// scheme (plaintext h2c into the TLS listener resets on the server
// preface) and present the openshell-client cert/key, or the gateway rejects the
// handshake and every dashboard->gateway call fails with 502 Unavailable.
func TestBuildConsoleDeployment_DashboardUsesGatewayMTLS(t *testing.T) {
	dep := buildConsoleDeployment("ns", "dash:latest", "proxy:latest",
		"https://issuer.example/realms/r", "gw-1-console", "https://console.example/oauth2/callback", false)

	dash := consoleContainerByName(t, dep, "dashboard")

	url, ok := envValue(dash, "OPENSHELL_GATEWAY_URL")
	if !ok || !strings.HasPrefix(url, "grpcs://") {
		t.Errorf("OPENSHELL_GATEWAY_URL = %q, want a grpcs:// TLS endpoint", url)
	}
	if v, _ := envValue(dash, "GATEWAY_CLIENT_CERT"); v != consoleGatewayClientCertPath {
		t.Errorf("GATEWAY_CLIENT_CERT = %q, want %q", v, consoleGatewayClientCertPath)
	}
	if v, _ := envValue(dash, "GATEWAY_CLIENT_KEY"); v != consoleGatewayClientKeyPath {
		t.Errorf("GATEWAY_CLIENT_KEY = %q, want %q", v, consoleGatewayClientKeyPath)
	}
	if !volumeNames(t, dep)["gateway-client"] {
		t.Error("expected gateway-client volume (openshell-client-tls) on the pod spec")
	}
	if !volumeMountNames(dash)["gateway-client"] {
		t.Error("expected gateway-client volumeMount on the dashboard container")
	}
}

// On OpenShift the restricted SCC assigns per-namespace UID/GID ranges and
// rejects a pod that pins fsGroup. Because the console is reconciled outside the
// gateway deploy path, reconcileConsole applies the same OpenShift fixup itself.
// Verify that fixup strips the hardcoded fsGroup while leaving the rest of the
// pod security context intact.
func TestConsoleDeployment_OpenShiftOverrideStripsFsGroup(t *testing.T) {
	dep := buildConsoleDeployment("ns", "dash:latest", "proxy:latest",
		"https://issuer.example/realms/r", "gw-1-console", "https://console.example/oauth2/callback", false)

	// Sanity: the base Deployment pins fsGroup off OpenShift.
	if _, found, _ := unstructured.NestedInt64(dep.Object, "spec", "template", "spec", "securityContext", "fsGroup"); !found {
		t.Fatal("expected the base console Deployment to set fsGroup")
	}

	applyOpenShiftOverrides(dep)

	if _, found, _ := unstructured.NestedInt64(dep.Object, "spec", "template", "spec", "securityContext", "fsGroup"); found {
		t.Error("expected fsGroup to be removed after the OpenShift override")
	}
	if v, found, _ := unstructured.NestedBool(dep.Object, "spec", "template", "spec", "securityContext", "runAsNonRoot"); !found || !v {
		t.Error("expected runAsNonRoot to remain true after the OpenShift override")
	}
}

// consoleRouteWithConditions builds a console HTTPRoute unstructured object whose
// single parent reports the given Accepted/ResolvedRefs condition statuses.
func consoleRouteWithConditions(namespace string, conditions []interface{}) *unstructured.Unstructured {
	route := &unstructured.Unstructured{}
	route.SetGroupVersionKind(consoleHTTPRouteGVR.GroupVersion().WithKind("HTTPRoute"))
	route.SetNamespace(namespace)
	route.SetName(consoleName)
	if conditions != nil {
		_ = unstructured.SetNestedSlice(route.Object, []interface{}{
			map[string]interface{}{
				"controllerName": "gateway.example/controller",
				"conditions":     conditions,
			},
		}, "status", "parents")
	}
	return route
}

func routeCondition(condType, status, reason string) map[string]interface{} {
	return map[string]interface{}{
		"type":   condType,
		"status": status,
		"reason": reason,
	}
}

func newConsoleRouteDynamicClient(objs ...runtime.Object) *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	gvrToListKind := map[schema.GroupVersionResource]string{
		consoleHTTPRouteGVR: "HTTPRouteList",
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, objs...)
}

// The console Deployment can be Ready while its HTTPRoute is rejected (e.g. a
// missing or misnamed HTTP listener on the shared Gateway). Publishing
// console_address then yields a dead "Open console" link, so readiness must
// require the route to be Accepted AND its backend refs resolved.
func TestConsoleRouteReady(t *testing.T) {
	ctx := context.Background()
	const ns = "openshell-abc"

	t.Run("accepted and resolved is ready", func(t *testing.T) {
		client := newConsoleRouteDynamicClient(consoleRouteWithConditions(ns, []interface{}{
			routeCondition("Accepted", "True", "Accepted"),
			routeCondition("ResolvedRefs", "True", "ResolvedRefs"),
		}))
		ready, reason, err := ConsoleRouteReady(ctx, client, ns)
		if err != nil {
			t.Fatalf("ConsoleRouteReady: %v", err)
		}
		if !ready {
			t.Errorf("expected ready; reason=%q", reason)
		}
	})

	t.Run("rejected listener is not ready and reports reason", func(t *testing.T) {
		client := newConsoleRouteDynamicClient(consoleRouteWithConditions(ns, []interface{}{
			routeCondition("Accepted", "False", "NoMatchingListenerHostname"),
			routeCondition("ResolvedRefs", "True", "ResolvedRefs"),
		}))
		ready, reason, err := ConsoleRouteReady(ctx, client, ns)
		if err != nil {
			t.Fatalf("ConsoleRouteReady: %v", err)
		}
		if ready {
			t.Error("expected not ready when the route is not Accepted")
		}
		if !strings.Contains(reason, "NoMatchingListenerHostname") {
			t.Errorf("reason = %q, want it to carry the listener rejection reason", reason)
		}
	})

	t.Run("unresolved backend refs is not ready", func(t *testing.T) {
		client := newConsoleRouteDynamicClient(consoleRouteWithConditions(ns, []interface{}{
			routeCondition("Accepted", "True", "Accepted"),
			routeCondition("ResolvedRefs", "False", "BackendNotFound"),
		}))
		ready, reason, err := ConsoleRouteReady(ctx, client, ns)
		if err != nil {
			t.Fatalf("ConsoleRouteReady: %v", err)
		}
		if ready {
			t.Error("expected not ready when backend refs are unresolved")
		}
		if !strings.Contains(reason, "BackendNotFound") {
			t.Errorf("reason = %q, want it to carry the unresolved-refs reason", reason)
		}
	})

	t.Run("no parent status yet is not ready", func(t *testing.T) {
		client := newConsoleRouteDynamicClient(consoleRouteWithConditions(ns, nil))
		ready, _, err := ConsoleRouteReady(ctx, client, ns)
		if err != nil {
			t.Fatalf("ConsoleRouteReady: %v", err)
		}
		if ready {
			t.Error("expected not ready when the route has no parent status")
		}
	})

	t.Run("missing route is not ready without error", func(t *testing.T) {
		client := newConsoleRouteDynamicClient()
		ready, reason, err := ConsoleRouteReady(ctx, client, ns)
		if err != nil {
			t.Fatalf("ConsoleRouteReady on missing route should not error: %v", err)
		}
		if ready {
			t.Error("expected not ready when the console HTTPRoute is absent")
		}
		if reason == "" {
			t.Error("expected a reason describing the missing route")
		}
	})
}

// In production the issuer is publicly trusted and the trusted-CA ConfigMap is
// absent, so the sidecar must not reference a CA file or mount that would fail
// to bind.
func TestBuildConsoleDeployment_NoTrustedCAOmitsOAuth2ProxyCA(t *testing.T) {
	dep := buildConsoleDeployment("ns", "dash:latest", "proxy:latest",
		"https://issuer.example/realms/r", "gw-1-console", "https://console.example/oauth2/callback", false)

	proxy := consoleContainerByName(t, dep, "oauth2-proxy")

	if _, ok := envValue(proxy, "OAUTH2_PROXY_PROVIDER_CA_FILES"); ok {
		t.Error("did not expect OAUTH2_PROXY_PROVIDER_CA_FILES when trustedCA is false")
	}
	if volumeNames(t, dep)["oidc-trusted-ca"] {
		t.Error("did not expect oidc-trusted-ca volume when trustedCA is false")
	}
	if volumeMountNames(proxy)["oidc-trusted-ca"] {
		t.Error("did not expect oidc-trusted-ca volumeMount when trustedCA is false")
	}
}

// The cookie secret must survive oauth2-proxy's SecretBytes decoding: strip
// padding, URL-base64-decode, and land on 16/24/32 bytes. A standard-base64
// value (44 chars, +/ alphabet) is rejected and used verbatim, crashing the
// sidecar with "cookie_secret must be 16, 24, or 32 bytes".
func TestConsoleURL(t *testing.T) {
	// With a base domain configured, the URL is https://console-<ns>.<domain>,
	// matching the address reconcileConsole builds and the reconciler publishes.
	t.Setenv("GATEWAY_API_BASE_DOMAIN", "gw.example.com")
	got, ok := ConsoleURL("openshell-abc")
	if !ok {
		t.Fatal("ConsoleURL: expected ok with base domain set")
	}
	if want := "https://console-openshell-abc.gw.example.com"; got != want {
		t.Errorf("ConsoleURL = %q, want %q", got, want)
	}

	// Without a base domain the console is disabled and the URL is unresolvable.
	t.Setenv("GATEWAY_API_BASE_DOMAIN", "")
	if _, ok := ConsoleURL("openshell-abc"); ok {
		t.Error("ConsoleURL: expected ok=false when base domain unset")
	}
}

func TestGenerateConsoleCookieSecret_DecodesTo32Bytes(t *testing.T) {
	for i := 0; i < 50; i++ {
		s, err := generateConsoleCookieSecret()
		if err != nil {
			t.Fatalf("generateConsoleCookieSecret: %v", err)
		}
		if strings.ContainsAny(s, "+/=") {
			t.Fatalf("cookie secret %q contains std-base64/padding chars oauth2-proxy cannot URL-decode", s)
		}
		decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
		if err != nil {
			t.Fatalf("cookie secret %q not RawURLEncoding-decodable: %v", s, err)
		}
		if n := len(decoded); n != 16 && n != 24 && n != 32 {
			t.Fatalf("cookie secret decodes to %d bytes, want 16/24/32", n)
		}
	}
}

// A brand-new tenant namespace has no console secret yet, so the first reconcile
// must CREATE it. Generating the cookie secret clears the local err, so the
// not-found state must be captured beforehand -- otherwise reconcile wrongly
// takes the Update path and fails with "secrets ... not found", aborting the
// whole console reconcile so no console is ever deployed. (The fake clientset
// does not run the server-side StringData->Data conversion, so assert on the
// StringData the code writes.)
func TestReconcileConsoleSecret_CreatesWhenAbsent(t *testing.T) {
	client := fake.NewSimpleClientset()

	if err := reconcileConsoleSecret(context.Background(), client, "openshell-ns", "client-abc"); err != nil {
		t.Fatalf("reconcileConsoleSecret on empty namespace: %v", err)
	}

	got, err := client.CoreV1().Secrets("openshell-ns").Get(context.Background(), consoleSecretName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected console secret to be created: %v", err)
	}
	if v := got.StringData["client-secret"]; v != "client-abc" {
		t.Errorf("client-secret = %q, want %q", v, "client-abc")
	}
	if got.StringData["cookie-secret"] == "" {
		t.Error("expected a generated cookie-secret")
	}
}

// On a subsequent reconcile the secret already exists and the cookie-secret must
// be preserved (so live browser sessions survive) while the client-secret is
// refreshed from Keycloak. Seed the existing secret's Data as a real API server
// would expose it (StringData is server-converted to Data on write).
func TestReconcileConsoleSecret_PreservesCookieOnUpdate(t *testing.T) {
	ctx := context.Background()
	existing := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: consoleSecretName, Namespace: "openshell-ns"},
		Type:       corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"client-secret": []byte("client-v1"),
			"cookie-secret": []byte("preserved-cookie-value"),
		},
	}
	client := fake.NewSimpleClientset(existing)

	if err := reconcileConsoleSecret(ctx, client, "openshell-ns", "client-v2"); err != nil {
		t.Fatalf("reconcile over existing secret: %v", err)
	}
	got, err := client.CoreV1().Secrets("openshell-ns").Get(ctx, consoleSecretName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if v := got.StringData["cookie-secret"]; v != "preserved-cookie-value" {
		t.Errorf("cookie-secret = %q, want preserved %q", v, "preserved-cookie-value")
	}
	if v := got.StringData["client-secret"]; v != "client-v2" {
		t.Errorf("client-secret = %q, want refreshed %q", v, "client-v2")
	}
}

// newConsoleIngressDynamicClient returns a fake dynamic client that knows both
// the console HTTPRoute (gateway-api mode) and the console OpenShift Route (route
// mode) list kinds, so reconcileConsoleIngress can create either.
func newConsoleIngressDynamicClient(objs ...runtime.Object) *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	gvrToListKind := map[schema.GroupVersionResource]string{
		consoleHTTPRouteGVR: "HTTPRouteList",
		consoleRouteGVR:     "RouteList",
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, objs...)
}

// P0-1/P0-2: the console ingress must follow the effective ingress mode. On a
// route-mode cluster GATEWAY_API_GATEWAY_NAME is unset, so emitting the
// gateway-api HTTPRoute unconditionally produced an object with an empty
// parentRef.name that the API server rejected and the self-heal loop retried
// forever. reconcileConsoleIngress now emits an OpenShift Route in route mode,
// an HTTPRoute (guarded against the empty name) in gateway-api mode, and nothing
// in none mode.
func TestReconcileConsoleIngress(t *testing.T) {
	ctx := context.Background()
	const ns = "openshell-abc"
	const host = "console-openshell-abc.apps.example.com"

	getUnstructured := func(t *testing.T, client *dynamicfake.FakeDynamicClient, gvr schema.GroupVersionResource) (*unstructured.Unstructured, error) {
		t.Helper()
		return client.Resource(gvr).Namespace(ns).Get(ctx, consoleName, metav1.GetOptions{})
	}

	t.Run("route mode creates an OpenShift Route, not an HTTPRoute", func(t *testing.T) {
		client := newConsoleIngressDynamicClient()
		if err := reconcileConsoleIngress(ctx, client, ns, host, IngressModeRoute); err != nil {
			t.Fatalf("reconcileConsoleIngress(route): %v", err)
		}
		route, err := getUnstructured(t, client, consoleRouteGVR)
		if err != nil {
			t.Fatalf("expected a console Route to be created: %v", err)
		}
		if h, _, _ := unstructured.NestedString(route.Object, "spec", "host"); h != host {
			t.Errorf("Route host = %q, want %q", h, host)
		}
		if term, _, _ := unstructured.NestedString(route.Object, "spec", "tls", "termination"); term != "edge" {
			t.Errorf("Route tls.termination = %q, want edge (oauth2-proxy serves plain HTTP)", term)
		}
		if tp, _, _ := unstructured.NestedString(route.Object, "spec", "port", "targetPort"); tp != "http" {
			t.Errorf("Route port.targetPort = %q, want http", tp)
		}
		if _, err := getUnstructured(t, client, consoleHTTPRouteGVR); err == nil {
			t.Error("route mode must NOT create a gateway-api HTTPRoute")
		}
	})

	t.Run("gateway-api mode with the gateway name set creates a valid HTTPRoute", func(t *testing.T) {
		t.Setenv("GATEWAY_API_GATEWAY_NAME", "shared-gw")
		client := newConsoleIngressDynamicClient()
		if err := reconcileConsoleIngress(ctx, client, ns, host, IngressModeGatewayAPI); err != nil {
			t.Fatalf("reconcileConsoleIngress(gateway-api): %v", err)
		}
		route, err := getUnstructured(t, client, consoleHTTPRouteGVR)
		if err != nil {
			t.Fatalf("expected a console HTTPRoute to be created: %v", err)
		}
		parents, _, _ := unstructured.NestedSlice(route.Object, "spec", "parentRefs")
		if len(parents) != 1 {
			t.Fatalf("expected 1 parentRef, got %d", len(parents))
		}
		name, _, _ := unstructured.NestedString(parents[0].(map[string]interface{}), "name")
		if name != "shared-gw" {
			t.Errorf("parentRef.name = %q, want the configured gateway name", name)
		}
	})

	// P0-2: fail fast instead of submitting an HTTPRoute with parentRef.name="".
	t.Run("gateway-api mode with the gateway name unset fails fast and creates nothing", func(t *testing.T) {
		t.Setenv("GATEWAY_API_GATEWAY_NAME", "")
		client := newConsoleIngressDynamicClient()
		err := reconcileConsoleIngress(ctx, client, ns, host, IngressModeGatewayAPI)
		if err == nil {
			t.Fatal("expected an error when GATEWAY_API_GATEWAY_NAME is unset in gateway-api mode")
		}
		if !strings.Contains(err.Error(), "GATEWAY_API_GATEWAY_NAME") {
			t.Errorf("error = %q, want it to name the missing variable", err)
		}
		if _, getErr := getUnstructured(t, client, consoleHTTPRouteGVR); getErr == nil {
			t.Error("no HTTPRoute must be created when the required gateway name is missing")
		}
	})

	t.Run("none mode creates neither ingress object", func(t *testing.T) {
		client := newConsoleIngressDynamicClient()
		if err := reconcileConsoleIngress(ctx, client, ns, host, IngressModeNone); err != nil {
			t.Fatalf("reconcileConsoleIngress(none): %v", err)
		}
		if _, err := getUnstructured(t, client, consoleRouteGVR); err == nil {
			t.Error("none mode must not create a Route")
		}
		if _, err := getUnstructured(t, client, consoleHTTPRouteGVR); err == nil {
			t.Error("none mode must not create an HTTPRoute")
		}
	})
}

// consoleOpenShiftRouteWithConditions builds a console OpenShift Route whose
// single status.ingress entry reports the given conditions.
func consoleOpenShiftRouteWithConditions(namespace string, conditions []interface{}) *unstructured.Unstructured {
	route := &unstructured.Unstructured{}
	route.SetGroupVersionKind(consoleRouteGVR.GroupVersion().WithKind("Route"))
	route.SetNamespace(namespace)
	route.SetName(consoleName)
	if conditions != nil {
		_ = unstructured.SetNestedSlice(route.Object, []interface{}{
			map[string]interface{}{
				"host":       "console.example.com",
				"conditions": conditions,
			},
		}, "status", "ingress")
	}
	return route
}

// A console Deployment can be Ready while its OpenShift Route is un-admitted
// (e.g. a host collision with another Route). Publishing console_address then
// yields a dead link, so route-mode readiness must require Admitted=True.
func TestConsoleOpenShiftRouteReady(t *testing.T) {
	ctx := context.Background()
	const ns = "openshell-abc"

	t.Run("admitted is ready", func(t *testing.T) {
		client := newConsoleIngressDynamicClient(consoleOpenShiftRouteWithConditions(ns, []interface{}{
			routeCondition("Admitted", "True", "Admitted"),
		}))
		ready, reason, err := ConsoleOpenShiftRouteReady(ctx, client, ns)
		if err != nil {
			t.Fatalf("ConsoleOpenShiftRouteReady: %v", err)
		}
		if !ready {
			t.Errorf("expected ready; reason=%q", reason)
		}
	})

	t.Run("un-admitted is not ready and reports reason", func(t *testing.T) {
		client := newConsoleIngressDynamicClient(consoleOpenShiftRouteWithConditions(ns, []interface{}{
			routeCondition("Admitted", "False", "HostAlreadyClaimed"),
		}))
		ready, reason, err := ConsoleOpenShiftRouteReady(ctx, client, ns)
		if err != nil {
			t.Fatalf("ConsoleOpenShiftRouteReady: %v", err)
		}
		if ready {
			t.Error("expected not ready when the Route is not Admitted")
		}
		if !strings.Contains(reason, "HostAlreadyClaimed") {
			t.Errorf("reason = %q, want it to carry the rejection reason", reason)
		}
	})

	t.Run("no ingress status yet is not ready", func(t *testing.T) {
		client := newConsoleIngressDynamicClient(consoleOpenShiftRouteWithConditions(ns, nil))
		ready, _, err := ConsoleOpenShiftRouteReady(ctx, client, ns)
		if err != nil {
			t.Fatalf("ConsoleOpenShiftRouteReady: %v", err)
		}
		if ready {
			t.Error("expected not ready when the Route has no ingress status")
		}
	})

	t.Run("missing route is not ready without error", func(t *testing.T) {
		client := newConsoleIngressDynamicClient()
		ready, reason, err := ConsoleOpenShiftRouteReady(ctx, client, ns)
		if err != nil {
			t.Fatalf("ConsoleOpenShiftRouteReady on missing route should not error: %v", err)
		}
		if ready {
			t.Error("expected not ready when the console Route is absent")
		}
		if reason == "" {
			t.Error("expected a reason describing the missing route")
		}
	})
}

// ConsoleExposureReady must dispatch to the readiness check matching the ingress
// mode so a route-mode console is not gated on a non-existent HTTPRoute (and vice
// versa), and treats "none" mode as ready (no managed ingress to gate on).
func TestConsoleExposureReady_DispatchesByMode(t *testing.T) {
	ctx := context.Background()
	const ns = "openshell-abc"

	t.Run("route mode reads the OpenShift Route", func(t *testing.T) {
		// Only the OpenShift Route exists and is admitted; the HTTPRoute is absent.
		client := newConsoleIngressDynamicClient(consoleOpenShiftRouteWithConditions(ns, []interface{}{
			routeCondition("Admitted", "True", "Admitted"),
		}))
		ready, _, err := ConsoleExposureReady(ctx, client, ns, IngressModeRoute)
		if err != nil {
			t.Fatalf("ConsoleExposureReady(route): %v", err)
		}
		if !ready {
			t.Error("route mode should report ready from the admitted OpenShift Route")
		}
	})

	t.Run("gateway-api mode reads the HTTPRoute", func(t *testing.T) {
		// Only the HTTPRoute exists and is accepted; the OpenShift Route is absent.
		client := newConsoleIngressDynamicClient(consoleRouteWithConditions(ns, []interface{}{
			routeCondition("Accepted", "True", "Accepted"),
			routeCondition("ResolvedRefs", "True", "ResolvedRefs"),
		}))
		ready, _, err := ConsoleExposureReady(ctx, client, ns, IngressModeGatewayAPI)
		if err != nil {
			t.Fatalf("ConsoleExposureReady(gateway-api): %v", err)
		}
		if !ready {
			t.Error("gateway-api mode should report ready from the accepted HTTPRoute")
		}
	})

	t.Run("none mode is ready with no managed ingress", func(t *testing.T) {
		client := newConsoleIngressDynamicClient()
		ready, _, err := ConsoleExposureReady(ctx, client, ns, IngressModeNone)
		if err != nil {
			t.Fatalf("ConsoleExposureReady(none): %v", err)
		}
		if !ready {
			t.Error("none mode has no managed console ingress and should report ready")
		}
	})
}

// buildConsoleRoute must terminate TLS at the router (edge) -- the oauth2-proxy
// sidecar serves plain HTTP -- and redirect cleartext callers so the OAuth
// cookie is never sent over HTTP.
func TestBuildConsoleRoute(t *testing.T) {
	const ns = "openshell-abc"
	const host = "console-openshell-abc.apps.example.com"
	route := buildConsoleRoute(ns, host)

	if kind := route.GetKind(); kind != "Route" {
		t.Errorf("kind = %q, want Route", kind)
	}
	if got, _, _ := unstructured.NestedString(route.Object, "spec", "host"); got != host {
		t.Errorf("host = %q, want %q", got, host)
	}
	if term, _, _ := unstructured.NestedString(route.Object, "spec", "tls", "termination"); term != "edge" {
		t.Errorf("tls.termination = %q, want edge", term)
	}
	if pol, _, _ := unstructured.NestedString(route.Object, "spec", "tls", "insecureEdgeTerminationPolicy"); pol != "Redirect" {
		t.Errorf("insecureEdgeTerminationPolicy = %q, want Redirect", pol)
	}
	if toName, _, _ := unstructured.NestedString(route.Object, "spec", "to", "name"); toName != consoleName {
		t.Errorf("to.name = %q, want %q", toName, consoleName)
	}
}
