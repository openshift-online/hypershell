package gateway

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"os"

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

// deriveConsoleHost returns the console hostname console-<ns>.<base-domain>. The
// bool is false when GATEWAY_API_BASE_DOMAIN is unset (the console address then
// stays empty, mirroring the routeAddress behaviour).
func deriveConsoleHost(namespace string) (string, bool) {
	baseDomain := os.Getenv("GATEWAY_API_BASE_DOMAIN")
	if baseDomain == "" {
		return "", false
	}
	return fmt.Sprintf("console-%s.%s", namespace, baseDomain), true
}

// ConsoleDeploymentName is the name of the per-gateway console Deployment
// (dashboard + oauth2-proxy sidecar). It is exported so the reconciler can
// observe the console pod's readiness before publishing the gateway's
// console_address, gating the web UI's console button on a servable console.
const ConsoleDeploymentName = consoleName

// ConsoleURL returns the external URL of the per-gateway console, mirroring the
// address reconcileConsole builds, or ok=false when GATEWAY_API_BASE_DOMAIN is
// unset (console disabled). The reconciler publishes this address only once the
// console Deployment is Ready.
func ConsoleURL(namespace string) (string, bool) {
	host, ok := deriveConsoleHost(namespace)
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

// consoleOpenShiftRouteGVR is the GroupVersionResource for the console Route.
var consoleOpenShiftRouteGVR = schema.GroupVersionResource{
	Group:    "route.openshift.io",
	Version:  "v1",
	Resource: "routes",
}

// ConsoleExposureReady reports the readiness of the console exposure for the
// selected ingress mode. A Gateway API HTTPRoute needs Accepted=True and
// ResolvedRefs=True on one parent. An OpenShift Route needs Admitted=True from
// one router. The reason describes a known rejection or an incomplete status.
func ConsoleExposureReady(ctx context.Context, dynamicClient dynamic.Interface, namespace, ingressMode string) (ready bool, reason string, err error) {
	switch ingressMode {
	case IngressModeGatewayAPI:
		return consoleHTTPRouteReady(ctx, dynamicClient, namespace)
	case IngressModeRoute:
		return consoleOpenShiftRouteReady(ctx, dynamicClient, namespace)
	case IngressModeNone:
		return false, "no console ingress mode selected", nil
	default:
		return false, "", fmt.Errorf("unsupported console ingress mode %q", ingressMode)
	}
}

// consoleHTTPRouteReady reports the readiness of the console HTTPRoute.
func consoleHTTPRouteReady(ctx context.Context, dynamicClient dynamic.Interface, namespace string) (ready bool, reason string, err error) {
	route, err := dynamicClient.Resource(consoleHTTPRouteGVR).Namespace(namespace).Get(ctx, consoleName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return false, "console HTTPRoute not found", nil
		}
		return false, "", fmt.Errorf("get console HTTPRoute %s/%s: %w", namespace, consoleName, err)
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

// consoleOpenShiftRouteReady reports the admission state of the console Route.
func consoleOpenShiftRouteReady(ctx context.Context, dynamicClient dynamic.Interface, namespace string) (ready bool, reason string, err error) {
	route, err := dynamicClient.Resource(consoleOpenShiftRouteGVR).Namespace(namespace).Get(ctx, consoleName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return false, "console OpenShift Route not found", nil
		}
		return false, "", fmt.Errorf("get console OpenShift Route %s/%s: %w", namespace, consoleName, err)
	}

	ingress, found, err := unstructured.NestedSlice(route.Object, "status", "ingress")
	if err != nil {
		return false, "", fmt.Errorf("read console OpenShift Route %s/%s status: %w", namespace, consoleName, err)
	}
	if !found || len(ingress) == 0 {
		return false, "console OpenShift Route has no router status yet", nil
	}

	const noAdmissionReason = "console OpenShift Route has no Admitted condition"
	reason = noAdmissionReason
	for _, item := range ingress {
		router, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		conditions, _, nestedErr := unstructured.NestedSlice(router, "conditions")
		if nestedErr != nil {
			return false, "", fmt.Errorf("read console OpenShift Route %s/%s ingress conditions: %w", namespace, consoleName, nestedErr)
		}
		for _, item := range conditions {
			condition, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			conditionType, _, _ := unstructured.NestedString(condition, "type")
			if conditionType != "Admitted" {
				continue
			}
			status, _, _ := unstructured.NestedString(condition, "status")
			if status == "True" {
				return true, "", nil
			}
			rejectionReason, _, _ := unstructured.NestedString(condition, "reason")
			if rejectionReason != "" {
				reason = fmt.Sprintf("console OpenShift Route not admitted: %s", rejectionReason)
			} else if reason == noAdmissionReason {
				reason = "console OpenShift Route not admitted: router rejected the route"
			}
		}
	}

	return false, reason, nil
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

// consoleListenerName returns the sectionName of the shared Gateway HTTP
// listener that console HTTPRoutes attach to (GATEWAY_API_HTTP_LISTENER_NAME,
// default "https").
func consoleListenerName() string {
	if n := os.Getenv("GATEWAY_API_HTTP_LISTENER_NAME"); n != "" {
		return n
	}
	return "https"
}

// ReconcileConsole idempotently reconciles the per-gateway console (Keycloak
// client, credential Secret, Deployment, Service, selected exposure, and
// NetworkPolicies) for an already-provisioned routed gateway. It exists so the
// continuous health reconciler can self-heal the console independently of the
// provisioning phase gate. A console failure does not stop the gateway. Thus,
// the provisioning path does not run again after the gateway reaches Running.
// The health reconciler repairs a transient failure or later drift. This
// function is a thin, exported wrapper over reconcileConsole that uses
// the default image set; callers pass the same ReconcileOpts fields the console
// reads (Keycloak, GatewayName, GatewayID, IsOpenShift, SkipNetworkPolicies).
func ReconcileConsole(ctx context.Context, dynamicClient dynamic.Interface, clientset *kubernetes.Clientset, nsConfig NamespaceConfig, opts ReconcileOpts) error {
	images := opts.Images
	if images == nil {
		images = StaticImageDefaults{}
	}
	return reconcileConsole(ctx, dynamicClient, clientset, nsConfig, opts, images)
}

// DeleteConsole removes the per-gateway console (Keycloak client, credential
// Secret, Deployment, Service, both exposure kinds, and NetworkPolicies). It
// clears the stored console_address through opts.UpdateConsoleAddress. It is the
// exported counterpart to ReconcileConsole. The continuous health reconciler
// can remove the console independently of the provisioning phase gate. This
// prevents a console and its Keycloak client from remaining after route removal.
// The function ignores absent resources, so each health check can call it. It
// returns all deletion errors so callers can retry partial cleanup.
func DeleteConsole(ctx context.Context, dynamicClient dynamic.Interface, clientset *kubernetes.Clientset, namespace string, opts ReconcileOpts) error {
	return deleteConsole(ctx, dynamicClient, clientset, namespace, opts)
}

// reconcileConsole deploys the per-gateway OpenShell dashboard and its
// oauth2-proxy sidecar. It uses the selected gateway ingress mode for the
// console exposure. Missing prerequisites are logged and skipped without a
// gateway reconciliation failure.
//
// The Keycloak work (client, mappers, secret) is treated as one atomic step:
// ProvisionConsoleClient rolls the client back if mapper creation fails, and a
// returned error stops before the workload is deployed, so a failed cycle leaves
// no partial console and retries on the next reconcile.
func reconcileConsole(ctx context.Context, dynamicClient dynamic.Interface, clientset *kubernetes.Clientset, nsConfig NamespaceConfig, opts ReconcileOpts, images ImageDefaults) error {
	namespace := nsConfig.Name

	ingressMode := gatewayIngressMode(opts)
	if ingressMode == IngressModeNone {
		log.Printf("WARN console: no ingress mode selected; skipping console for namespace %s", namespace)
		return nil
	}
	if opts.Keycloak == nil {
		log.Printf("WARN console: Keycloak not configured; skipping console for namespace %s", namespace)
		return nil
	}
	if opts.GatewayName == "" || opts.GatewayID == "" {
		log.Printf("WARN console: gateway name and ID are required; skipping console for namespace %s", namespace)
		return nil
	}

	host, ok := deriveConsoleHost(namespace)
	if !ok {
		log.Printf("WARN console: GATEWAY_API_BASE_DOMAIN not set; skipping console for namespace %s", namespace)
		return nil
	}
	consoleURL := "https://" + host
	gwClientID := fmt.Sprintf("%s-%s", opts.GatewayName, opts.GatewayID)
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
		log.Printf("INFO provisioned console client %s", consoleClientID)
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
		log.Printf("INFO console client %s already exists (uuid=%s), config reconciled", consoleClientID, existingUUID)
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

	consoleImage := images.DefaultConsoleImage()
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

	if err := reconcileConsoleExposure(ctx, dynamicClient, namespace, host, ingressMode); err != nil {
		return fmt.Errorf("reconcile console exposure in %s: %w", namespace, err)
	}

	if opts.SkipNetworkPolicies {
		logNetworkPoliciesDisabled()
	} else {
		for _, np := range buildConsoleNetworkPolicies(namespace) {
			if err := reconcileResource(ctx, dynamicClient, np); err != nil {
				log.Printf("WARN failed to reconcile console NetworkPolicy %s: %v", np.GetName(), err)
			}
		}
	}

	// The console_address is deliberately NOT published here. Publishing it at
	// apply time surfaces the web UI's console button before the oauth2-proxy and
	// dashboard pod can serve, so a user clicking it hits a not-ready console. The
	// reconciler package publishes console_address only once this Deployment is
	// observed Ready (and retracts it if the pod later goes unready), gating the
	// button on a servable console. See openshell-gateway-console.spec.md.

	log.Printf("INFO console reconciled in namespace %s (host=%s)", namespace, host)
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
		log.Printf("INFO created console secret %s in %s", consoleSecretName, namespace)
		return nil
	}

	secret.ResourceVersion = existing.ResourceVersion
	if _, err := clientset.CoreV1().Secrets(namespace).Update(ctx, secret, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update console secret: %w", err)
	}
	log.Printf("INFO updated console secret %s in %s", consoleSecretName, namespace)
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
		"limits":   map[string]interface{}{"cpu": "500m", "memory": "128Mi"},
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
// kubelet can probe it, but the Service and NetworkPolicies keep 8000
// unreachable so only the in-pod oauth2-proxy reaches it. When trustedCA is set,
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

// buildConsoleHTTPRoute builds the HTTPRoute attaching the console Service to the
// shared Gateway HTTP listener at console-<ns>.<base-domain>.
func buildConsoleHTTPRoute(namespace, host string) *unstructured.Unstructured {
	parentRef := map[string]interface{}{
		"name":        gatewayIngressName(),
		"namespace":   gatewayIngressNamespace(),
		"sectionName": consoleListenerName(),
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

// buildConsoleOpenShiftRoute builds the edge-terminated Route for the console.
// The OpenShift router sends HTTP traffic to the named Service port.
func buildConsoleOpenShiftRoute(namespace, host string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "route.openshift.io/v1",
			"kind":       "Route",
			"metadata": map[string]interface{}{
				"name":      consoleName,
				"namespace": namespace,
				"labels":    consoleLabelsAny(),
			},
			"spec": map[string]interface{}{
				"host": host,
				"to": map[string]interface{}{
					"kind": "Service",
					"name": consoleName,
				},
				"port": map[string]interface{}{
					"targetPort": "http",
				},
				"tls": map[string]interface{}{
					"termination":                   "edge",
					"insecureEdgeTerminationPolicy": "Redirect",
				},
			},
		},
	}
}

// reconcileConsoleExposure creates only the exposure for the selected mode.
// It first removes the inactive exposure to prevent two controllers from
// claiming the same host during a mode change.
func reconcileConsoleExposure(ctx context.Context, dynamicClient dynamic.Interface, namespace, host, ingressMode string) error {
	var desired *unstructured.Unstructured
	var inactiveGVR schema.GroupVersionResource
	var inactiveKind string

	switch ingressMode {
	case IngressModeGatewayAPI:
		desired = buildConsoleHTTPRoute(namespace, host)
		inactiveGVR = consoleOpenShiftRouteGVR
		inactiveKind = "OpenShift Route"
	case IngressModeRoute:
		desired = buildConsoleOpenShiftRoute(namespace, host)
		inactiveGVR = consoleHTTPRouteGVR
		inactiveKind = "HTTPRoute"
	default:
		return fmt.Errorf("unsupported console ingress mode %q", ingressMode)
	}

	if err := deleteConsoleExposureResource(ctx, dynamicClient, namespace, inactiveGVR, inactiveKind); err != nil {
		return fmt.Errorf("remove inactive exposure: %w", err)
	}
	if err := reconcileResource(ctx, dynamicClient, desired); err != nil {
		return fmt.Errorf("reconcile selected %s exposure: %w", ingressMode, err)
	}
	return nil
}

// deleteConsoleExposureResource deletes one console exposure. An absent
// resource is already converged and does not cause an error.
func deleteConsoleExposureResource(ctx context.Context, dynamicClient dynamic.Interface, namespace string, gvr schema.GroupVersionResource, kind string) error {
	err := dynamicClient.Resource(gvr).Namespace(namespace).Delete(ctx, consoleName, metav1.DeleteOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("delete console %s in %s: %w", kind, namespace, err)
	}
	return nil
}

// deleteConsoleExposures removes both exposure kinds. It attempts both deletes
// so cleanup can converge after an ingress mode change.
func deleteConsoleExposures(ctx context.Context, dynamicClient dynamic.Interface, namespace string) error {
	return errors.Join(
		deleteConsoleExposureResource(ctx, dynamicClient, namespace, consoleHTTPRouteGVR, "HTTPRoute"),
		deleteConsoleExposureResource(ctx, dynamicClient, namespace, consoleOpenShiftRouteGVR, "OpenShift Route"),
	)
}

// buildConsoleNetworkPolicies builds the two console NetworkPolicies: ingress to
// oauth2-proxy from the shared Gateway namespace, and ingress to the gateway pod
// from the console pod.
func buildConsoleNetworkPolicies(namespace string) []*unstructured.Unstructured {
	gwNS := gatewayIngressNamespace()

	allowRouter := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.k8s.io/v1",
			"kind":       "NetworkPolicy",
			"metadata": map[string]interface{}{
				"name":      "openshell-console-allow-router",
				"namespace": namespace,
				"labels":    consoleLabelsAny(),
			},
			"spec": map[string]interface{}{
				"podSelector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"app.kubernetes.io/instance": consoleName,
						"app.kubernetes.io/name":     "openshell",
					},
				},
				"policyTypes": []interface{}{"Ingress"},
				"ingress": []interface{}{
					map[string]interface{}{
						"ports": []interface{}{
							map[string]interface{}{"port": consoleProxyPort, "protocol": "TCP"},
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
					},
				},
			},
		},
	}

	allowConsole := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.k8s.io/v1",
			"kind":       "NetworkPolicy",
			"metadata": map[string]interface{}{
				"name":      "openshell-gateway-allow-console",
				"namespace": namespace,
				"labels":    consoleLabelsAny(),
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
						"ports": []interface{}{
							map[string]interface{}{"port": int64(8080), "protocol": "TCP"},
						},
						"from": []interface{}{
							map[string]interface{}{
								"podSelector": map[string]interface{}{
									"matchLabels": map[string]interface{}{
										"app.kubernetes.io/instance": consoleName,
										"app.kubernetes.io/name":     "openshell",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	return []*unstructured.Unstructured{allowRouter, allowConsole}
}

// deleteConsole removes all console resources and the console Keycloak client.
// It attempts every deletion regardless of individual failures and returns their
// joined errors (nil when the console is already absent), so a caller can retry
// until nothing remains instead of stopping on partial cleanup. A NotFound is not
// an error: the resource is already gone.
func deleteConsole(ctx context.Context, dynamicClient dynamic.Interface, clientset *kubernetes.Clientset, namespace string, opts ReconcileOpts) error {
	var errs []error

	deployGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	if err := dynamicClient.Resource(deployGVR).Namespace(namespace).Delete(ctx, consoleName, metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("delete console Deployment in %s: %w", namespace, err))
	}

	if err := clientset.CoreV1().Services(namespace).Delete(ctx, consoleName, metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("delete console Service in %s: %w", namespace, err))
	}

	if err := deleteConsoleExposures(ctx, dynamicClient, namespace); err != nil {
		errs = append(errs, err)
	}

	netpolGVR := schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}
	for _, name := range []string{"openshell-console-allow-router", "openshell-gateway-allow-console"} {
		if err := dynamicClient.Resource(netpolGVR).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
			errs = append(errs, fmt.Errorf("delete console NetworkPolicy %s in %s: %w", name, namespace, err))
		}
	}

	if err := clientset.CoreV1().Secrets(namespace).Delete(ctx, consoleSecretName, metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("delete console Secret in %s: %w", namespace, err))
	}

	if opts.Keycloak != nil && opts.GatewayName != "" && opts.GatewayID != "" {
		consoleClientID := fmt.Sprintf("%s-%s-console", opts.GatewayName, opts.GatewayID)
		kc := keycloak.NewClient(
			opts.Keycloak.ServerURL,
			opts.Keycloak.Realm,
			opts.Keycloak.ClientID,
			opts.Keycloak.ClientSecret,
		)
		if err := kc.DeleteConsoleClient(ctx, consoleClientID); err != nil {
			errs = append(errs, fmt.Errorf("delete console client %s (orphaned): %w", consoleClientID, err))
		} else {
			log.Printf("INFO deleted console client %s", consoleClientID)
		}
	}

	if opts.UpdateConsoleAddress != nil {
		if err := opts.UpdateConsoleAddress(ctx, ""); err != nil {
			errs = append(errs, fmt.Errorf("clear consoleAddress in %s: %w", namespace, err))
		} else {
			log.Printf("INFO cleared consoleAddress for gateway in %s", namespace)
		}
	}

	return errors.Join(errs...)
}
