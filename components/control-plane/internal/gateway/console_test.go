package gateway

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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

// consoleHTTPRouteWithConditions builds a console HTTPRoute with one parent.
func consoleHTTPRouteWithConditions(namespace string, conditions []interface{}) *unstructured.Unstructured {
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

// consoleOpenShiftRouteWithConditions builds a console Route with one router.
func consoleOpenShiftRouteWithConditions(namespace string, conditions []interface{}) *unstructured.Unstructured {
	route := &unstructured.Unstructured{}
	route.SetGroupVersionKind(consoleOpenShiftRouteGVR.GroupVersion().WithKind("Route"))
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
		consoleHTTPRouteGVR:      "HTTPRouteList",
		consoleOpenShiftRouteGVR: "RouteList",
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, objs...)
}

// The console Deployment can be Ready while its HTTPRoute is rejected (e.g. a
// missing or misnamed HTTP listener on the shared Gateway). Publishing
// console_address then yields a dead "Open console" link, so readiness must
// require the route to be Accepted AND its backend refs resolved.
func TestConsoleExposureReadyHTTPRoute(t *testing.T) {
	ctx := context.Background()
	const ns = "openshell-abc"

	t.Run("accepted and resolved is ready", func(t *testing.T) {
		client := newConsoleRouteDynamicClient(consoleHTTPRouteWithConditions(ns, []interface{}{
			routeCondition("Accepted", "True", "Accepted"),
			routeCondition("ResolvedRefs", "True", "ResolvedRefs"),
		}))
		ready, reason, err := ConsoleExposureReady(ctx, client, ns, IngressModeGatewayAPI)
		if err != nil {
			t.Fatalf("ConsoleExposureReady: %v", err)
		}
		if !ready {
			t.Errorf("expected ready; reason=%q", reason)
		}
	})

	t.Run("rejected listener is not ready and reports reason", func(t *testing.T) {
		client := newConsoleRouteDynamicClient(consoleHTTPRouteWithConditions(ns, []interface{}{
			routeCondition("Accepted", "False", "NoMatchingListenerHostname"),
			routeCondition("ResolvedRefs", "True", "ResolvedRefs"),
		}))
		ready, reason, err := ConsoleExposureReady(ctx, client, ns, IngressModeGatewayAPI)
		if err != nil {
			t.Fatalf("ConsoleExposureReady: %v", err)
		}
		if ready {
			t.Error("expected not ready when the route is not Accepted")
		}
		if !strings.Contains(reason, "NoMatchingListenerHostname") {
			t.Errorf("reason = %q, want it to carry the listener rejection reason", reason)
		}
	})

	t.Run("unresolved backend refs is not ready", func(t *testing.T) {
		client := newConsoleRouteDynamicClient(consoleHTTPRouteWithConditions(ns, []interface{}{
			routeCondition("Accepted", "True", "Accepted"),
			routeCondition("ResolvedRefs", "False", "BackendNotFound"),
		}))
		ready, reason, err := ConsoleExposureReady(ctx, client, ns, IngressModeGatewayAPI)
		if err != nil {
			t.Fatalf("ConsoleExposureReady: %v", err)
		}
		if ready {
			t.Error("expected not ready when backend refs are unresolved")
		}
		if !strings.Contains(reason, "BackendNotFound") {
			t.Errorf("reason = %q, want it to carry the unresolved-refs reason", reason)
		}
	})

	t.Run("no parent status yet is not ready", func(t *testing.T) {
		client := newConsoleRouteDynamicClient(consoleHTTPRouteWithConditions(ns, nil))
		ready, _, err := ConsoleExposureReady(ctx, client, ns, IngressModeGatewayAPI)
		if err != nil {
			t.Fatalf("ConsoleExposureReady: %v", err)
		}
		if ready {
			t.Error("expected not ready when the route has no parent status")
		}
	})

	t.Run("missing route is not ready without error", func(t *testing.T) {
		client := newConsoleRouteDynamicClient()
		ready, reason, err := ConsoleExposureReady(ctx, client, ns, IngressModeGatewayAPI)
		if err != nil {
			t.Fatalf("ConsoleExposureReady on missing route should not error: %v", err)
		}
		if ready {
			t.Error("expected not ready when the console HTTPRoute is absent")
		}
		if reason == "" {
			t.Error("expected a reason describing the missing route")
		}
	})
}

func TestConsoleExposureReadyOpenShiftRoute(t *testing.T) {
	ctx := context.Background()
	const ns = "openshell-abc"

	t.Run("admitted route is ready", func(t *testing.T) {
		client := newConsoleRouteDynamicClient(consoleOpenShiftRouteWithConditions(ns, []interface{}{
			routeCondition("Admitted", "True", "RouteAdmitted"),
		}))
		ready, reason, err := ConsoleExposureReady(ctx, client, ns, IngressModeRoute)
		if err != nil {
			t.Fatalf("ConsoleExposureReady: %v", err)
		}
		if !ready {
			t.Errorf("expected ready; reason=%q", reason)
		}
	})

	t.Run("rejected route reports the admission reason", func(t *testing.T) {
		client := newConsoleRouteDynamicClient(consoleOpenShiftRouteWithConditions(ns, []interface{}{
			routeCondition("Admitted", "False", "HostAlreadyClaimed"),
		}))
		ready, reason, err := ConsoleExposureReady(ctx, client, ns, IngressModeRoute)
		if err != nil {
			t.Fatalf("ConsoleExposureReady: %v", err)
		}
		if ready {
			t.Fatal("expected a rejected Route to be not ready")
		}
		if !strings.Contains(reason, "HostAlreadyClaimed") {
			t.Errorf("reason = %q, want the admission reason", reason)
		}
	})

	t.Run("one admitted router makes the route ready", func(t *testing.T) {
		route := consoleOpenShiftRouteWithConditions(ns, []interface{}{
			routeCondition("Admitted", "False", "HostAlreadyClaimed"),
		})
		_ = unstructured.SetNestedSlice(route.Object, []interface{}{
			map[string]interface{}{
				"host": "console.example.com",
				"conditions": []interface{}{
					routeCondition("Admitted", "False", "HostAlreadyClaimed"),
				},
			},
			map[string]interface{}{
				"host": "console.example.com",
				"conditions": []interface{}{
					routeCondition("Admitted", "True", "RouteAdmitted"),
				},
			},
		}, "status", "ingress")

		client := newConsoleRouteDynamicClient(route)
		ready, reason, err := ConsoleExposureReady(ctx, client, ns, IngressModeRoute)
		if err != nil {
			t.Fatalf("ConsoleExposureReady: %v", err)
		}
		if !ready {
			t.Errorf("expected ready; reason=%q", reason)
		}
	})

	t.Run("a generic rejection keeps an earlier specific reason", func(t *testing.T) {
		route := consoleOpenShiftRouteWithConditions(ns, nil)
		_ = unstructured.SetNestedSlice(route.Object, []interface{}{
			map[string]interface{}{
				"conditions": []interface{}{
					routeCondition("Admitted", "False", "HostAlreadyClaimed"),
				},
			},
			map[string]interface{}{
				"conditions": []interface{}{
					routeCondition("Admitted", "False", ""),
				},
			},
		}, "status", "ingress")

		client := newConsoleRouteDynamicClient(route)
		ready, reason, err := ConsoleExposureReady(ctx, client, ns, IngressModeRoute)
		if err != nil {
			t.Fatalf("ConsoleExposureReady: %v", err)
		}
		if ready {
			t.Fatal("expected a rejected Route to be not ready")
		}
		if !strings.Contains(reason, "HostAlreadyClaimed") {
			t.Errorf("reason = %q, want the specific admission reason", reason)
		}
	})

	t.Run("route without router status is not ready", func(t *testing.T) {
		client := newConsoleRouteDynamicClient(consoleOpenShiftRouteWithConditions(ns, nil))
		ready, reason, err := ConsoleExposureReady(ctx, client, ns, IngressModeRoute)
		if err != nil {
			t.Fatalf("ConsoleExposureReady: %v", err)
		}
		if ready || reason == "" {
			t.Errorf("ready = %v, reason = %q; want not ready with a reason", ready, reason)
		}
	})

	t.Run("missing route is not ready", func(t *testing.T) {
		client := newConsoleRouteDynamicClient()
		ready, reason, err := ConsoleExposureReady(ctx, client, ns, IngressModeRoute)
		if err != nil {
			t.Fatalf("ConsoleExposureReady: %v", err)
		}
		if ready || reason == "" {
			t.Errorf("ready = %v, reason = %q; want not ready with a reason", ready, reason)
		}
	})
}

func TestBuildConsoleOpenShiftRoute(t *testing.T) {
	const (
		namespace = "openshell-abc"
		host      = "console-openshell-abc.apps.example.com"
	)
	route := buildConsoleOpenShiftRoute(namespace, host)

	if route.GetAPIVersion() != "route.openshift.io/v1" || route.GetKind() != "Route" {
		t.Errorf("route GVK = %s %s, want route.openshift.io/v1 Route", route.GetAPIVersion(), route.GetKind())
	}
	if route.GetName() != consoleName || route.GetNamespace() != namespace {
		t.Errorf("route identity = %s/%s, want %s/%s", route.GetNamespace(), route.GetName(), namespace, consoleName)
	}
	if got := route.GetLabels()["app.kubernetes.io/component"]; got != "console" {
		t.Errorf("component label = %q, want console", got)
	}

	checks := []struct {
		name string
		path []string
		want string
	}{
		{name: "host", path: []string{"spec", "host"}, want: host},
		{name: "service kind", path: []string{"spec", "to", "kind"}, want: "Service"},
		{name: "service name", path: []string{"spec", "to", "name"}, want: consoleName},
		{name: "target port", path: []string{"spec", "port", "targetPort"}, want: "http"},
		{name: "TLS termination", path: []string{"spec", "tls", "termination"}, want: "edge"},
		{name: "insecure policy", path: []string{"spec", "tls", "insecureEdgeTerminationPolicy"}, want: "Redirect"},
	}
	for _, check := range checks {
		got, found, err := unstructured.NestedString(route.Object, check.path...)
		if err != nil || !found || got != check.want {
			t.Errorf("%s = %q, found=%v, err=%v; want %q", check.name, got, found, err, check.want)
		}
	}
}

func TestReconcileConsoleExposureSelectsOneMode(t *testing.T) {
	const (
		namespace = "openshell-abc"
		host      = "console-openshell-abc.apps.example.com"
	)

	tests := []struct {
		name        string
		mode        string
		desiredGVR  schema.GroupVersionResource
		inactiveGVR schema.GroupVersionResource
		inactive    runtime.Object
	}{
		{
			name:        "Gateway API mode",
			mode:        IngressModeGatewayAPI,
			desiredGVR:  consoleHTTPRouteGVR,
			inactiveGVR: consoleOpenShiftRouteGVR,
			inactive:    buildConsoleOpenShiftRoute(namespace, host),
		},
		{
			name:        "OpenShift Route mode",
			mode:        IngressModeRoute,
			desiredGVR:  consoleOpenShiftRouteGVR,
			inactiveGVR: consoleHTTPRouteGVR,
			inactive:    buildConsoleHTTPRoute(namespace, host),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := newConsoleRouteDynamicClient(test.inactive)
			for pass := 1; pass <= 2; pass++ {
				if err := reconcileConsoleExposure(context.Background(), client, namespace, host, test.mode); err != nil {
					t.Fatalf("reconcileConsoleExposure pass %d: %v", pass, err)
				}
			}

			if _, err := client.Resource(test.desiredGVR).Namespace(namespace).Get(context.Background(), consoleName, metav1.GetOptions{}); err != nil {
				t.Errorf("selected exposure is absent: %v", err)
			}
			if _, err := client.Resource(test.inactiveGVR).Namespace(namespace).Get(context.Background(), consoleName, metav1.GetOptions{}); !k8serrors.IsNotFound(err) {
				t.Errorf("inactive exposure still exists: %v", err)
			}
		})
	}
}

func TestDeleteConsoleExposuresDeletesBothKinds(t *testing.T) {
	const namespace = "openshell-abc"
	client := newConsoleRouteDynamicClient(
		buildConsoleHTTPRoute(namespace, "console.example.com"),
		buildConsoleOpenShiftRoute(namespace, "console.example.com"),
	)

	if err := deleteConsoleExposures(context.Background(), client, namespace); err != nil {
		t.Fatalf("deleteConsoleExposures: %v", err)
	}
	for _, gvr := range []schema.GroupVersionResource{consoleHTTPRouteGVR, consoleOpenShiftRouteGVR} {
		if _, err := client.Resource(gvr).Namespace(namespace).Get(context.Background(), consoleName, metav1.GetOptions{}); !k8serrors.IsNotFound(err) {
			t.Errorf("%s exposure still exists: %v", gvr.Resource, err)
		}
	}
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
