package gateway

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"

	"github.com/openshift-online/hypershell/components/control-plane/internal/exposure"
	"github.com/openshift-online/hypershell/components/control-plane/internal/keycloak"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

const (
	consoleName          = "openshell-console"
	consoleSecretName    = "openshell-console-oauth2"
	consoleDashboardPort = int64(8000)
	consoleProxyPort     = int64(4180)

	// consoleTrustedCAConfigMap is the per-namespace ConfigMap the control plane
	// copies from its own namespace (reconcileTrustedCABundle) holding the CA that
	// signs the OIDC issuer's certificate. The gateway pod trusts it the same way.
	consoleTrustedCAConfigMap = "gateway-trusted-ca"
	consoleTrustedCAKey       = "ca-bundle.crt"
	consoleTrustedCAMountPath = "/etc/openshell-tls/oidc/ca-bundle.crt"

	// The dashboard dials the gateway's admin gRPC API over mutual TLS: it
	// verifies the gateway with the openshell CA (openshell-server-tls/ca.crt)
	// and authenticates itself with the openshell-client cert/key
	// (openshell-client-tls). Both secrets are provisioned per namespace by the
	// cert-manager reconcile (reconcileCertificates).
	consoleGatewayCACertPath     = "/etc/openshell-tls/gateway/ca.crt"
	consoleGatewayClientCertPath = "/etc/openshell-tls/gateway-client/tls.crt"
	consoleGatewayClientKeyPath  = "/etc/openshell-tls/gateway-client/tls.key"
)

// consoleLabels returns the standard gateway labels for console resources, with
// the console-specific component and instance labels.
func consoleLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "openshell",
		"app.kubernetes.io/component":  "console",
		"app.kubernetes.io/instance":   consoleName,
		"app.kubernetes.io/managed-by": "hypershell-control-plane",
		"hypershell.redhat.io/managed": "true",
	}
}

// consoleLabelsAny returns consoleLabels as a map[string]interface{} for use in
// unstructured objects.
func consoleLabelsAny() map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range consoleLabels() {
		out[k] = v
	}
	return out
}

// deriveConsoleHost returns the console hostname for a tenant namespace:
// console-<ns>.<base-domain> for gateways on the shared ingress, and
// console-<ns>.gw-<ns>.<base-domain> for gateways opted in to the dedicated
// per-gateway certificate (Route.Certificate). The opted-in host nests the
// console label one DNS label under the gateway's own derived hostname --
// derived through exposure.DeriveGatewayAPIHost, the single source of truth
// for the gateway hostname, so the two can never drift -- which places the
// console inside the per-tenant Gateway's wildcard *.gw-<ns> listener and its
// wildcard certificate SAN, so the console needs no certificate of its own.
// The bool is false when GATEWAY_API_BASE_DOMAIN is unset (the console address
// then stays empty, mirroring the routeAddress behaviour).
func deriveConsoleHost(nsConfig NamespaceConfig) (string, bool) {
	if nsConfig.Gateway.Route.Certificate != nil {
		gatewayHost, ok := exposure.DeriveGatewayAPIHost(nsConfig.Name, "")
		if !ok {
			return "", false
		}
		return fmt.Sprintf("console-%s.%s", nsConfig.Name, gatewayHost), true
	}
	baseDomain := os.Getenv("GATEWAY_API_BASE_DOMAIN")
	if baseDomain == "" {
		return "", false
	}
	return fmt.Sprintf("console-%s.%s", nsConfig.Name, baseDomain), true
}

// ConsoleDeploymentName is the name of the per-gateway console Deployment
// (dashboard + oauth2-proxy sidecar). It is exported so the reconciler can
// observe the console pod's readiness before publishing the gateway's
// console_address, gating the web UI's console button on a servable console.
const ConsoleDeploymentName = consoleName

// ConsoleURLFor returns the external URL of the per-gateway console for the
// given namespace config, mirroring the address reconcileConsole builds, or
// ok=false when the console host cannot be derived (GATEWAY_API_BASE_DOMAIN
// unset, console disabled). Gateways opted in to the dedicated per-gateway
// certificate serve from the per-tenant Gateway's own domain
// (console-<ns>.gw-<ns>.<base-domain>); callers that know the gateway's parsed
// route config must use this form so the published address matches the host
// the console is actually served on.
func ConsoleURLFor(nsConfig NamespaceConfig) (string, bool) {
	host, ok := deriveConsoleHost(nsConfig)
	if !ok {
		return "", false
	}
	return "https://" + host, true
}

// consoleHTTPRouteGVR is the GroupVersionResource for the console HTTPRoute.
var consoleHTTPRouteGVR = schema.GroupVersionResource{
	Group:    "gateway.networking.k8s.io",
	Version:  "v1",
	Resource: "httproutes",
}

// ConsoleExposureReady reports the readiness of the console HTTPRoute, AND that the exposure serves the console host derived
// from the gateway's route config. A Gateway API HTTPRoute needs Accepted=True
// and ResolvedRefs=True on one parent, its hostname must be the derived console
// host, and its parentRef must target the Gateway that host is served by (the
// per-tenant Gateway's "sandbox" listener for the dedicated per-gateway
// certificate opt-in, the shared Gateway's HTTP listener otherwise). The
// reason describes a known rejection, an
// incomplete status, or a host/parent mismatch (the console self-heal converges
// the exposure when one is reported).
//
// The host and parent checks are what keep the published console_address
// honest: a console exposure that is Accepted but serves a different host --
// for example a dedicated-route gateway whose console was provisioned onto
// the shared ingress, whose dedicated host then resolves (through the shared
// wildcard DNS) to a Gateway with no listener for it -- would otherwise
// advertise a URL that presents a mismatched certificate and is refused by
// every browser.
func ConsoleExposureReady(ctx context.Context, dynamicClient dynamic.Interface, nsConfig NamespaceConfig) (ready bool, reason string, err error) {
	host, ok := deriveConsoleHost(nsConfig)
	if !ok {
		return false, "console host cannot be derived (GATEWAY_API_BASE_DOMAIN unset)", nil
	}
	return consoleHTTPRouteReady(ctx, dynamicClient, nsConfig, host)
}

// consoleHTTPRouteReady reports the readiness of the console HTTPRoute for
// the derived console host: the spec must serve that host on the serving
// Gateway's listener (consoleHTTPRouteServesExpectedHost), and one parent must
// have fully accepted the route. The most specific rejection reason across
// parents is tracked for the not-ready case.
func consoleHTTPRouteReady(ctx context.Context, dynamicClient dynamic.Interface, nsConfig NamespaceConfig, host string) (ready bool, reason string, err error) {
	namespace := nsConfig.Name
	route, err := dynamicClient.Resource(consoleHTTPRouteGVR).Namespace(namespace).Get(ctx, consoleName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return false, "console HTTPRoute not found", nil
		}
		return false, "", fmt.Errorf("get console HTTPRoute %s/%s: %w", namespace, consoleName, err)
	}

	if reason := consoleHTTPRouteServesExpectedHost(route, nsConfig, host); reason != "" {
		return false, reason, nil
	}

	parents, found, err := unstructured.NestedSlice(route.Object, "status", "parents")
	if err != nil {
		return false, "", fmt.Errorf("read console HTTPRoute %s/%s status: %w", namespace, consoleName, err)
	}
	if !found || len(parents) == 0 {
		return false, "console HTTPRoute has no parent status yet", nil
	}

	// Report ready as soon as any parent has fully accepted the route. Track the
	// most specific rejection reason across parents for the not-ready case.
	reason = "console HTTPRoute not accepted"
	for _, p := range parents {
		parent, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		conditions, _, _ := unstructured.NestedSlice(parent, "conditions")
		accepted, acceptedReason := routeConditionState(conditions, "Accepted")
		resolved, resolvedReason := routeConditionState(conditions, "ResolvedRefs")
		if accepted && resolved {
			return true, "", nil
		}
		if !accepted && acceptedReason != "" {
			reason = fmt.Sprintf("console HTTPRoute not accepted: %s", acceptedReason)
		} else if !resolved && resolvedReason != "" {
			reason = fmt.Sprintf("console HTTPRoute backend refs unresolved: %s", resolvedReason)
		}
	}
	return false, reason, nil
}

// consoleHTTPRouteServesExpectedHost reports whether the console HTTPRoute's
// spec matches the exposure the derived console host requires: the hostname
// list must contain the derived host, and one parentRef must target the
// Gateway that serves it -- the per-tenant Gateway's "sandbox" listener for a
// dedicated-certificate gateway, the shared Gateway's HTTP listener otherwise
// (the same shapes buildConsoleHTTPRoute emits). An empty return means the
// spec matches; otherwise it is the actionable not-ready reason. The console
// self-heal converges the exposure when a mismatch is reported, so the reason
// names both what is served and what was expected.
func consoleHTTPRouteServesExpectedHost(route *unstructured.Unstructured, nsConfig NamespaceConfig, host string) string {
	hostnames, _, err := unstructured.NestedStringSlice(route.Object, "spec", "hostnames")
	if err != nil {
		return "console HTTPRoute spec.hostnames is unreadable"
	}
	servesHost := false
	for _, h := range hostnames {
		if h == host {
			servesHost = true
			break
		}
	}
	if !servesHost {
		return fmt.Sprintf("console HTTPRoute serves %v, expected hostname %q (console self-heal re-parents it onto the derived host)", hostnames, host)
	}

	parentRefs, _, err := unstructured.NestedSlice(route.Object, "spec", "parentRefs")
	if err != nil {
		return "console HTTPRoute spec.parentRefs is unreadable"
	}

	if nsConfig.Gateway.Route.Certificate != nil {
		// The dedicated path never rides the shared Gateway: the console must
		// parent on the per-tenant Gateway (gw-<namespace>) in its own
		// namespace, on the wildcard "sandbox" listener whose hostname covers
		// the console host and whose certificate is the per-gateway one.
		for _, p := range parentRefs {
			ref, ok := p.(map[string]interface{})
			if !ok {
				continue
			}
			name, _, _ := unstructured.NestedString(ref, "name")
			ns, _, _ := unstructured.NestedString(ref, "namespace")
			section, _, _ := unstructured.NestedString(ref, "sectionName")
			if name == perTenantGatewayName(nsConfig.Name) && ns == nsConfig.Name && section == "sandbox" {
				return ""
			}
		}
		return fmt.Sprintf("console HTTPRoute is not parented on the per-tenant Gateway %s/%s listener \"sandbox\" (console self-heal re-parents it)", nsConfig.Name, perTenantGatewayName(nsConfig.Name))
	}

	// Shared path: the console parents on the shared Gateway's HTTP listener.
	// The shared Gateway name is empty only on a controller without one
	// configured; then the listener alone identifies the expected parent.
	expectedListener := sharedGatewayListenerName()
	for _, p := range parentRefs {
		ref, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		section, _, _ := unstructured.NestedString(ref, "sectionName")
		if section != expectedListener {
			continue
		}
		if name := gatewayIngressName(); name != "" {
			n, _, _ := unstructured.NestedString(ref, "name")
			if n != name {
				continue
			}
			ns, _, _ := unstructured.NestedString(ref, "namespace")
			if ns != gatewayIngressNamespace() {
				continue
			}
		}
		return ""
	}
	return fmt.Sprintf("console HTTPRoute is not parented on the shared Gateway listener %q (console self-heal re-parents it)", expectedListener)
}

// routeConditionState returns whether the named HTTPRoute parent condition is
// True, along with its Reason when it is not True (empty when the condition is
// absent).
func routeConditionState(conditions []interface{}, condType string) (isTrue bool, reason string) {
	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if t, _, _ := unstructured.NestedString(cond, "type"); t != condType {
			continue
		}
		status, _, _ := unstructured.NestedString(cond, "status")
		if status == "True" {
			return true, ""
		}
		r, _, _ := unstructured.NestedString(cond, "reason")
		return false, r
	}
	return false, ""
}

// sharedGatewayListenerName returns the sectionName of the shared Gateway
// listener that both the gateway's GRPCRoute and the per-gateway console's
// HTTPRoute attach to (GATEWAY_API_HTTP_LISTENER_NAME, default "https"). Both
// route kinds must resolve to the same listener, so this is the single source
// of truth for both call sites -- a mismatch here reproduces as GRPCRoute or
// HTTPRoute status NoMatchingParent even when the Gateway itself is Programmed.
func sharedGatewayListenerName() string {
	if n := os.Getenv("GATEWAY_API_HTTP_LISTENER_NAME"); n != "" {
		return n
	}
	return "https"
}

// ReconcileConsole idempotently reconciles the per-gateway console (Keycloak
// client, credential Secret, Deployment, Service, and selected exposure) for an
// already-provisioned routed gateway. It exists so the continuous health
// reconciler can self-heal the console independently of the provisioning phase
// gate. A console failure does not stop the gateway. Thus, the provisioning
// path does not run again after the gateway reaches Running. The health
// reconciler repairs a transient failure or later drift. This function is a
// thin, exported wrapper over reconcileConsole that uses the default image set;
// callers pass the same ReconcileOpts fields the console reads (Keycloak,
// GatewayName, GatewayID, IsOpenShift).
func ReconcileConsole(ctx context.Context, dynamicClient dynamic.Interface, clientset *kubernetes.Clientset, nsConfig NamespaceConfig, opts ReconcileOpts) error {
	images := opts.Images
	if images == nil {
		images = StaticImageDefaults{}
	}
	return reconcileConsole(ctx, dynamicClient, clientset, nsConfig, opts, images)
}

// reconcileConsole deploys the per-gateway OpenShell dashboard and its
// oauth2-proxy sidecar, exposed through a Gateway API HTTPRoute. Missing prerequisites are logged and skipped without a
// gateway reconciliation failure.
//
// The Keycloak work (client, mappers, secret) is treated as one atomic step:
// ProvisionConsoleClient rolls the client back if mapper creation fails, and a
// returned error stops before the workload is deployed, so a failed cycle leaves
// no partial console and retries on the next reconcile.
func reconcileConsole(ctx context.Context, dynamicClient dynamic.Interface, clientset *kubernetes.Clientset, nsConfig NamespaceConfig, opts ReconcileOpts, images ImageDefaults) error {
	namespace := nsConfig.Name

	if opts.Keycloak == nil {
		return fmt.Errorf("console in %s: keycloak configuration is required", namespace)
	}
	if opts.GatewayClientID == "" || opts.GatewayID == "" {
		slog.WarnContext(ctx, "Console: gateway client ID and gateway ID are required; skipping console", "namespace", namespace)
		return nil
	}
	// Check the image before any Keycloak or cluster writes, so a missing
	// image does not leave a half-provisioned console behind.
	consoleImage := images.DefaultConsoleImage()
	if consoleImage == "" {
		return fmt.Errorf("console image is not configured for namespace %s; set %s", namespace, ConsoleImageEnv)
	}

	host, ok := deriveConsoleHost(nsConfig)
	if !ok {
		slog.WarnContext(ctx, "Console: GATEWAY_API_BASE_DOMAIN not set; skipping console", "namespace", namespace)
		return nil
	}
	consoleURL := "https://" + host
	gwClientID := opts.GatewayClientID
	consoleClientID := gwClientID + "-console"
	redirectURI := consoleURL + "/oauth2/callback"

	kc := keycloak.NewClient(
		opts.Keycloak.ServerURL,
		opts.Keycloak.Realm,
		opts.Keycloak.ClientID,
		opts.Keycloak.ClientSecret,
	)

	existingUUID, err := kc.GetClientUUID(ctx, consoleClientID)
	if err != nil {
		return fmt.Errorf("check console client %s: %w", consoleClientID, err)
	}
	var clientSecret string
	if existingUUID == "" {
		_, clientSecret, err = kc.ProvisionConsoleClient(ctx, consoleClientID, gwClientID, redirectURI, consoleURL)
		if err != nil {
			return fmt.Errorf("provision console client %s: %w", consoleClientID, err)
		}
		slog.InfoContext(ctx, "Provisioned console client", "client_id", consoleClientID)
	} else {
		clientSecret, err = kc.GetConsoleClientSecret(ctx, consoleClientID)
		if err != nil {
			return fmt.Errorf("get console client secret %s: %w", consoleClientID, err)
		}
		// Reconcile the full desired client configuration on every pass so a
		// console provisioned before a given setting existed (or one that has since
		// drifted) is healed: redirect URIs and web origins (which change if the
		// base domain changes), fullScopeAllowed=false and the confidential flags,
		// the protocol mappers, and the gateway-client scope mappings. The gateway
		// client roles must be in the console client's scope or Keycloak strips
		// hypershell.roles and the gateway denies every call.
		if err = kc.EnsureConsoleClientConfig(ctx, consoleClientID, gwClientID, redirectURI, consoleURL); err != nil {
			return fmt.Errorf("ensure console client config for %s: %w", consoleClientID, err)
		}
		slog.InfoContext(ctx, "Console client already exists, config reconciled", "client_id", consoleClientID, "uuid", existingUUID)
	}

	// The oauth2-proxy issuer must match the issuer the gateway validates, so the
	// browser token `iss` claim is accepted. Mirror the gateway client issuer
	// override (GATEWAY_OIDC_ISSUER_URL) used in reconcileKeycloakClient.
	issuer := kc.Issuer()
	if v := os.Getenv("GATEWAY_OIDC_ISSUER_URL"); v != "" {
		issuer = v
	}

	// The credential Secret must exist before the Deployment references it.
	if err := reconcileConsoleSecret(ctx, clientset, namespace, clientSecret); err != nil {
		return fmt.Errorf("reconcile console secret in %s: %w", namespace, err)
	}

	proxyImage := images.DefaultOAuth2ProxyImage()

	// oauth2-proxy performs OIDC discovery against the issuer over HTTPS. When the
	// issuer is served with a privately-signed certificate (the gateway pod faces
	// the same problem), the control plane copies the trusting CA into every
	// tenant namespace as the gateway-trusted-ca ConfigMap. Mount it into the
	// sidecar so discovery succeeds. In production the issuer presents a
	// publicly-trusted certificate, the ConfigMap is absent, and the sidecar
	// falls back to the system trust store -- config, not code, drives this.
	trustedCA := hasConsoleTrustedCABundle(ctx, clientset, namespace)

	deployment := buildConsoleDeployment(namespace, consoleImage, proxyImage, issuer, consoleClientID, redirectURI, trustedCA)
	// Console resources are reconciled outside deployGateway, so the gateway
	// path's OpenShift fixup never reaches this Deployment. The restricted SCC
	// assigns namespace-specific UID/GID ranges and rejects a hardcoded fsGroup,
	// so strip it (and any pinned container runAsUser) on OpenShift, matching the
	// gateway path.
	if opts.IsOpenShift {
		applyOpenShiftOverrides(deployment)
	}
	if err := reconcileResource(ctx, dynamicClient, deployment); err != nil {
		return fmt.Errorf("reconcile console Deployment in %s: %w", namespace, err)
	}

	if err := reconcileResource(ctx, dynamicClient, buildConsoleService(namespace)); err != nil {
		return fmt.Errorf("reconcile console Service in %s: %w", namespace, err)
	}

	if err := reconcileConsoleExposure(ctx, dynamicClient, nsConfig, host); err != nil {
		return fmt.Errorf("reconcile console exposure in %s: %w", namespace, err)
	}

	// The console_address is deliberately NOT published here. Publishing it at
	// apply time surfaces the web UI's console button before the oauth2-proxy and
	// dashboard pod can serve, so a user clicking it hits a not-ready console. The
	// reconciler package publishes console_address only once this Deployment is
	// observed Ready (and retracts it if the pod later goes unready), gating the
	// button on a servable console. See openshell-gateway-console.spec.md.

	// The log names where the console parents: the per-tenant Gateway's
	// sandbox listener for the per-gateway certificate opt-in, the shared
	// Gateway's HTTP listener otherwise.
	if nsConfig.Gateway.Route.Certificate != nil {
		slog.InfoContext(ctx, "Console reconciled",
			"namespace", namespace,
			"host", host,
			"gateway", "gw-"+namespace,
			"listener", "sandbox",
		)
	} else {
		slog.InfoContext(ctx, "Console reconciled", "namespace", namespace, "host", host, "listener", sharedGatewayListenerName())
	}
	return nil
}

// generateConsoleCookieSecret returns a fresh oauth2-proxy cookie secret.
// oauth2-proxy strips padding and URL-base64-decodes the value, then requires
// 16/24/32 decoded bytes. Standard base64 (with +/ and = padding) fails that
// decode and is used verbatim as a 44-byte string, which oauth2-proxy rejects
// for AES. RawURLEncoding of 32 bytes yields a 43-char, padding-free, URL-safe
// string that decodes back to 32 bytes.
func generateConsoleCookieSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate cookie secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// reconcileConsoleSecret creates or updates the openshell-console-oauth2 Secret.
// The cookie-secret is generated once and preserved across reconciles so active
// browser sessions stay valid; the client-secret is refreshed from Keycloak.
// Neither value is ever written to a log.
func reconcileConsoleSecret(ctx context.Context, clientset kubernetes.Interface, namespace, clientSecret string) error {
	existing, err := clientset.CoreV1().Secrets(namespace).Get(ctx, consoleSecretName, metav1.GetOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("get console secret: %w", err)
	}
	// Capture existence before err is reused below; generating the cookie clears
	// err on success, which would otherwise mask the NotFound and send a
	// brand-new namespace down the Update path (secret ... not found).
	secretExists := err == nil

	cookieSecret := ""
	if secretExists {
		if v, ok := existing.Data["cookie-secret"]; ok && len(v) > 0 {
			cookieSecret = string(v)
		}
	}
	if cookieSecret == "" {
		cookieSecret, err = generateConsoleCookieSecret()
		if err != nil {
			return err
		}
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      consoleSecretName,
			Namespace: namespace,
			Labels:    consoleLabels(),
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"client-secret": clientSecret,
			"cookie-secret": cookieSecret,
		},
	}

	if !secretExists {
		if _, err := clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create console secret: %w", err)
		}
		slog.InfoContext(ctx, "Created console secret", "secret", consoleSecretName, "namespace", namespace)
		return nil
	}

	secret.ResourceVersion = existing.ResourceVersion
	if _, err := clientset.CoreV1().Secrets(namespace).Update(ctx, secret, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update console secret: %w", err)
	}
	slog.InfoContext(ctx, "Updated console secret", "secret", consoleSecretName, "namespace", namespace)
	return nil
}

// consoleSecurityContext returns the restricted container securityContext shared
// by both console containers.
func consoleSecurityContext() map[string]interface{} {
	return map[string]interface{}{
		"runAsNonRoot":             true,
		"allowPrivilegeEscalation": false,
		"readOnlyRootFilesystem":   true,
		"seccompProfile":           map[string]interface{}{"type": "RuntimeDefault"},
		"capabilities":             map[string]interface{}{"drop": []interface{}{"ALL"}},
	}
}

// consoleResources returns modest resource requests/limits for a console container.
func consoleResources() map[string]interface{} {
	return map[string]interface{}{
		"requests": map[string]interface{}{"cpu": "10m", "memory": "64Mi"},
		"limits":   map[string]interface{}{"memory": "128Mi"},
	}
}

func envVar(name, value string) map[string]interface{} {
	return map[string]interface{}{"name": name, "value": value}
}

func envFromSecret(name, secretName, key string) map[string]interface{} {
	return map[string]interface{}{
		"name": name,
		"valueFrom": map[string]interface{}{
			"secretKeyRef": map[string]interface{}{
				"name": secretName,
				"key":  key,
			},
		},
	}
}

// hasConsoleTrustedCABundle reports whether the per-namespace trusted-CA
// ConfigMap (copied by reconcileTrustedCABundle) is present, meaning the OIDC
// issuer is served with a privately-signed certificate the oauth2-proxy sidecar
// must be told to trust. Absent in production, where the issuer is publicly
// trusted and the sidecar uses the system trust store.
func hasConsoleTrustedCABundle(ctx context.Context, clientset *kubernetes.Clientset, namespace string) bool {
	if clientset == nil {
		return false
	}
	_, err := clientset.CoreV1().ConfigMaps(namespace).Get(ctx, consoleTrustedCAConfigMap, metav1.GetOptions{})
	return err == nil
}

// buildConsoleDeployment builds the two-container console Deployment (dashboard +
// oauth2-proxy sidecar). The dashboard binds all interfaces on 8000 so the
// kubelet can probe it, but the Service keeps 8000 internal so only
// the in-pod oauth2-proxy reaches it. When trustedCA is set,
// the sidecar mounts the tenant's gateway-trusted-ca bundle and is pointed at it
// for OIDC discovery so a privately-signed issuer certificate validates.
func buildConsoleDeployment(namespace, consoleImage, proxyImage, issuer, consoleClientID, redirectURI string, trustedCA bool) *unstructured.Unstructured {
	selectorLabels := map[string]interface{}{
		"app.kubernetes.io/name":     "openshell",
		"app.kubernetes.io/instance": consoleName,
	}

	dashboardContainer := map[string]interface{}{
		"name":            "dashboard",
		"image":           consoleImage,
		"imagePullPolicy": "IfNotPresent",
		"env": []interface{}{
			envVar("PORT", fmt.Sprintf("%d", consoleDashboardPort)),
			// The grpcs:// scheme is required for the dashboard's gRPC client to
			// dial over TLS; without it the client speaks h2c cleartext into the
			// gateway's TLS listener and the connection is reset while reading the
			// server preface. The gateway's admin API also requires mutual TLS, so
			// the dashboard presents the openshell-client cert/key below.
			envVar("OPENSHELL_GATEWAY_URL", fmt.Sprintf("grpcs://openshell-gateway.%s.svc.cluster.local:8080", namespace)),
			envVar("GATEWAY_CA_CERT", consoleGatewayCACertPath),
			envVar("GATEWAY_CLIENT_CERT", consoleGatewayClientCertPath),
			envVar("GATEWAY_CLIENT_KEY", consoleGatewayClientKeyPath),
			envVar("AUTH_DISABLED", "false"),
			envVar("AUTH_TOKEN_HEADER", "X-Forwarded-Access-Token"),
			envVar("AUTH_USER_HEADER", "X-Forwarded-User"),
			envVar("ADMIN_ROLE", "openshell-admin"),
			envVar("LOGOUT_URL", "/oauth2/sign_out"),
		},
		"ports": []interface{}{
			map[string]interface{}{"containerPort": consoleDashboardPort, "name": "dashboard"},
		},
		"readinessProbe": map[string]interface{}{
			"tcpSocket":           map[string]interface{}{"port": consoleDashboardPort},
			"initialDelaySeconds": int64(5),
			"periodSeconds":       int64(10),
		},
		"volumeMounts": []interface{}{
			map[string]interface{}{"name": "gateway-ca", "mountPath": "/etc/openshell-tls/gateway", "readOnly": true},
			map[string]interface{}{"name": "gateway-client", "mountPath": "/etc/openshell-tls/gateway-client", "readOnly": true},
			map[string]interface{}{"name": "tmp-dashboard", "mountPath": "/tmp"},
		},
		"securityContext": consoleSecurityContext(),
		"resources":       consoleResources(),
	}

	proxyEnv := []interface{}{
		envVar("OAUTH2_PROXY_PROVIDER", "oidc"),
		envVar("OAUTH2_PROXY_OIDC_ISSUER_URL", issuer),
		envVar("OAUTH2_PROXY_CLIENT_ID", consoleClientID),
		envFromSecret("OAUTH2_PROXY_CLIENT_SECRET", consoleSecretName, "client-secret"),
		envFromSecret("OAUTH2_PROXY_COOKIE_SECRET", consoleSecretName, "cookie-secret"),
		envVar("OAUTH2_PROXY_CODE_CHALLENGE_METHOD", "S256"),
		envVar("OAUTH2_PROXY_REDIRECT_URL", redirectURI),
		envVar("OAUTH2_PROXY_UPSTREAMS", fmt.Sprintf("http://127.0.0.1:%d", consoleDashboardPort)),
		envVar("OAUTH2_PROXY_HTTP_ADDRESS", fmt.Sprintf("0.0.0.0:%d", consoleProxyPort)),
		envVar("OAUTH2_PROXY_REVERSE_PROXY", "true"),
		envVar("OAUTH2_PROXY_PASS_ACCESS_TOKEN", "true"),
		envVar("OAUTH2_PROXY_PASS_USER_HEADERS", "true"),
		envVar("OAUTH2_PROXY_SKIP_PROVIDER_BUTTON", "true"),
		envVar("OAUTH2_PROXY_COOKIE_SECURE", "true"),
		envVar("OAUTH2_PROXY_COOKIE_REFRESH", "2m"),
		envVar("OAUTH2_PROXY_EMAIL_DOMAINS", "*"),
		envVar("OAUTH2_PROXY_SCOPE", "openid profile email roles"),
	}
	proxyVolumeMounts := []interface{}{
		map[string]interface{}{"name": "tmp-proxy", "mountPath": "/tmp"},
	}
	if trustedCA {
		// oauth2-proxy appends these CA files to the system trust store for its
		// OIDC discovery/token requests, so the privately-signed issuer validates.
		proxyEnv = append(proxyEnv, envVar("OAUTH2_PROXY_PROVIDER_CA_FILES", consoleTrustedCAMountPath))
		proxyVolumeMounts = append(proxyVolumeMounts, map[string]interface{}{
			"name":      "oidc-trusted-ca",
			"mountPath": consoleTrustedCAMountPath,
			"subPath":   consoleTrustedCAKey,
			"readOnly":  true,
		})
	}

	proxyContainer := map[string]interface{}{
		"name":            "oauth2-proxy",
		"image":           proxyImage,
		"imagePullPolicy": "IfNotPresent",
		"env":             proxyEnv,
		"ports": []interface{}{
			map[string]interface{}{"containerPort": consoleProxyPort, "name": "http"},
		},
		"readinessProbe": map[string]interface{}{
			"httpGet":             map[string]interface{}{"path": "/ready", "port": consoleProxyPort},
			"initialDelaySeconds": int64(5),
			"periodSeconds":       int64(10),
		},
		"livenessProbe": map[string]interface{}{
			"httpGet":             map[string]interface{}{"path": "/ping", "port": consoleProxyPort},
			"initialDelaySeconds": int64(10),
			"periodSeconds":       int64(20),
		},
		"volumeMounts":    proxyVolumeMounts,
		"securityContext": consoleSecurityContext(),
		"resources":       consoleResources(),
	}

	volumes := []interface{}{
		map[string]interface{}{
			"name": "gateway-ca",
			"secret": map[string]interface{}{
				"secretName": "openshell-server-tls",
				"items": []interface{}{
					map[string]interface{}{"key": "ca.crt", "path": "ca.crt"},
				},
			},
		},
		map[string]interface{}{
			"name": "gateway-client",
			"secret": map[string]interface{}{
				"secretName": "openshell-client-tls",
				"items": []interface{}{
					map[string]interface{}{"key": "tls.crt", "path": "tls.crt"},
					map[string]interface{}{"key": "tls.key", "path": "tls.key"},
				},
			},
		},
		map[string]interface{}{"name": "tmp-dashboard", "emptyDir": map[string]interface{}{}},
		map[string]interface{}{"name": "tmp-proxy", "emptyDir": map[string]interface{}{}},
	}
	if trustedCA {
		volumes = append(volumes, map[string]interface{}{
			"name": "oidc-trusted-ca",
			"configMap": map[string]interface{}{
				"name": consoleTrustedCAConfigMap,
				"items": []interface{}{
					map[string]interface{}{"key": consoleTrustedCAKey, "path": consoleTrustedCAKey},
				},
			},
		})
	}

	podSpec := map[string]interface{}{
		"securityContext": map[string]interface{}{
			"runAsNonRoot":   true,
			"fsGroup":        int64(1001),
			"seccompProfile": map[string]interface{}{"type": "RuntimeDefault"},
		},
		"containers": []interface{}{dashboardContainer, proxyContainer},
		"volumes":    volumes,
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      consoleName,
				"namespace": namespace,
				"labels":    consoleLabelsAny(),
			},
			"spec": map[string]interface{}{
				"replicas": int64(1),
				"selector": map[string]interface{}{"matchLabels": selectorLabels},
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{"labels": consoleLabelsAny()},
					"spec":     podSpec,
				},
			},
		},
	}
}

// buildConsoleService builds the ClusterIP Service that exposes only the
// oauth2-proxy port (4180). Port 8000 (dashboard) is intentionally not exposed.
func buildConsoleService(namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      consoleName,
				"namespace": namespace,
				"labels":    consoleLabelsAny(),
			},
			"spec": map[string]interface{}{
				"type": "ClusterIP",
				"selector": map[string]interface{}{
					"app.kubernetes.io/name":     "openshell",
					"app.kubernetes.io/instance": consoleName,
				},
				"ports": []interface{}{
					map[string]interface{}{
						"name":       "http",
						"port":       consoleProxyPort,
						"targetPort": consoleProxyPort,
						"protocol":   "TCP",
					},
				},
			},
		},
	}
}

// buildConsoleHTTPRoute builds the HTTPRoute attaching the console Service to
// an HTTP listener at the console host. Gateways on the shared ingress parent
// onto the shared Gateway's listener; gateways opted in to the dedicated
// per-gateway certificate parent onto their per-tenant Gateway (gw-<namespace>,
// created in the tenant namespace by the reconciler) and its wildcard
// "sandbox" listener, whose hostname *.gw-<namespace>.<base-domain> covers the
// console's host. parentRefs cannot span the two: the sectionName and the
// Gateway must match, or the route reports NoMatchingParent.
func buildConsoleHTTPRoute(namespace, host string, perGatewayCertificate bool) *unstructured.Unstructured {
	var parentRef map[string]interface{}
	if perGatewayCertificate {
		// The per-tenant Gateway lives in the tenant namespace itself, so the
		// parentRef namespace is the console's own. The name is always the
		// derived gw-<namespace>: the opt-in path never rides the cluster's
		// shared Gateway (GATEWAY_API_GATEWAY_NAME).
		parentRef = map[string]interface{}{
			"name":        "gw-" + namespace,
			"namespace":   namespace,
			"sectionName": "sandbox",
		}
	} else {
		parentRef = map[string]interface{}{
			"name":        gatewayIngressName(),
			"namespace":   gatewayIngressNamespace(),
			"sectionName": sharedGatewayListenerName(),
		}
	}
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "HTTPRoute",
			"metadata": map[string]interface{}{
				"name":      consoleName,
				"namespace": namespace,
				"labels":    consoleLabelsAny(),
			},
			"spec": map[string]interface{}{
				"parentRefs": []interface{}{parentRef},
				"hostnames":  []interface{}{host},
				"rules": []interface{}{
					map[string]interface{}{
						"backendRefs": []interface{}{
							map[string]interface{}{
								"name": consoleName,
								"port": consoleProxyPort,
							},
						},
					},
				},
			},
		},
	}
}

// reconcileConsoleExposure creates the console HTTPRoute.
func reconcileConsoleExposure(ctx context.Context, dynamicClient dynamic.Interface, nsConfig NamespaceConfig, host string) error {
	desired := buildConsoleHTTPRoute(nsConfig.Name, host, nsConfig.Gateway.Route.Certificate != nil)
	if err := reconcileResource(ctx, dynamicClient, desired); err != nil {
		return fmt.Errorf("reconcile console HTTPRoute: %w", err)
	}
	return nil
}
