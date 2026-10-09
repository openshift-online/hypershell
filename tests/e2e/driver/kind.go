package driver

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-online/hypershell/tests/e2e/apiclient"
	"github.com/openshift-online/hypershell/tests/e2e/harness"
)

// Kind driver defaults. These match the Kind deploy overlay (deploy/kind) and the
// seeded local environment, and are overridable through the E2E_* environment
// variables the spec documents.
const (
	kindDriverName = "kind"

	kindDefaultPlatformNS   = "hypershell-system"
	kindDefaultKeycloakNS   = "keycloak"
	kindDefaultOIDCIssuer   = "https://keycloak.hypershell.localhost/realms/hypershell"
	kindDefaultAPIHost      = "https://api.hypershell.localhost"
	kindDefaultConsoleHost  = "https://console.hypershell.localhost"
	kindDefaultClusterDom   = "gw.localhost"
	kindDefaultIngressAddr  = "127.0.0.1:443"
	kindDefaultFrontendID   = "hypershell-frontend"
	kindCASecretName        = "hypershell-ca-secret"
	kindCASecretKey         = "ca.crt"
	kindControllerDeploy    = "hypershell-controller"
	kindControllerContainer = "controller"

	// Keycloak admin (master realm) bootstrap, for the role-assignment helpers.
	kindDefaultKCAdminUser     = "admin"
	kindDefaultKCAdminPassword = "admin"
	kcAdminCLIClient           = "admin-cli"
	kcMasterRealm              = "master"

	gcEnvInterval       = "GATEWAY_NAMESPACE_GC_INTERVAL"
	gcEnvGrace          = "GATEWAY_NAMESPACE_GC_GRACE_PERIOD"
	directoryRefreshEnv = "GATEWAY_DIRECTORY_REFRESH_INTERVAL"
)

func init() {
	Register(kindDriverName, func(c *harness.Clients) (E2EInfraDriver, error) {
		return newKindDriver(c)
	})
}

// kindDriver implements E2EInfraDriver against a Kind cluster: ingress hostnames
// resolved through Gateway API routes, the Kind self-signed CA, and Keycloak
// reached at keycloak.hypershell.localhost. Host resolution is handled by dialing
// the forwarded cluster ingress (ingressAddr), since the *.localhost names are not
// in DNS on the host running the suite.
type kindDriver struct {
	clients *harness.Clients

	platformNS    string
	keycloakNS    string
	oidcIssuer    string
	apiHost       string
	consoleHost   string
	clusterDomain string
	frontendID    string
	ingressAddr   string

	kcAdminUser     string
	kcAdminPassword string

	caPool     *x509.CertPool
	httpClient *http.Client

	gcPatched bool
}

func newKindDriver(clients *harness.Clients) (*kindDriver, error) {
	d := &kindDriver{
		clients:         clients,
		platformNS:      envOr("E2E_NAMESPACE", kindDefaultPlatformNS),
		keycloakNS:      envOr("E2E_KEYCLOAK_NAMESPACE", kindDefaultKeycloakNS),
		oidcIssuer:      envOr("E2E_OIDC_ISSUER", kindDefaultOIDCIssuer),
		apiHost:         envOr("E2E_API_HOST", kindDefaultAPIHost),
		consoleHost:     envOr("E2E_CONSOLE_URL", kindDefaultConsoleHost),
		clusterDomain:   envOr("GATEWAY_API_BASE_DOMAIN", kindDefaultClusterDom),
		frontendID:      envOr("E2E_OIDC_CLIENT_ID", kindDefaultFrontendID),
		ingressAddr:     envOr("E2E_KIND_INGRESS_ADDR", kindDefaultIngressAddr),
		kcAdminUser:     envOr("E2E_KC_ADMIN_USER", kindDefaultKCAdminUser),
		kcAdminPassword: envOr("E2E_KC_ADMIN_PASSWORD", kindDefaultKCAdminPassword),
	}

	// Extract the Kind CA from the in-cluster secret so the suite trusts the
	// cluster's self-signed TLS for the API server, Keycloak, and the console,
	// with no insecure bypass.
	caPEM, err := d.fetchClusterCA(context.Background())
	if err != nil {
		return nil, fmt.Errorf("extract cluster CA: %w", err)
	}
	pool, err := harness.BuildCAPool(caPEM)
	if err != nil {
		return nil, fmt.Errorf("build CA pool: %w", err)
	}
	d.caPool = pool

	rewrite := d.hostRewriter()
	if err := harness.InstallDefaultCA(pool, rewrite); err != nil {
		return nil, fmt.Errorf("install cluster CA into default transport: %w", err)
	}
	d.httpClient = harness.HTTPClientWithCA(pool, rewrite, 30*time.Second)

	return d, nil
}

// hostRewriter redirects every *.localhost hostname (the Kind ingress and gateway
// endpoints) to the forwarded cluster ingress address, so the suite can reach them
// without host DNS entries.
func (d *kindDriver) hostRewriter() harness.HostRewriter {
	return func(host string) (string, bool) {
		if host == "localhost" || strings.HasSuffix(host, ".localhost") {
			return d.ingressAddr, true
		}
		return "", false
	}
}

func (d *kindDriver) fetchClusterCA(ctx context.Context) ([]byte, error) {
	secret, err := d.clients.Kube.CoreV1().Secrets(d.platformNS).Get(ctx, kindCASecretName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get secret %s/%s: %w", d.platformNS, kindCASecretName, err)
	}
	ca, ok := secret.Data[kindCASecretKey]
	if !ok || len(ca) == 0 {
		return nil, fmt.Errorf("secret %s/%s has no %q", d.platformNS, kindCASecretName, kindCASecretKey)
	}
	return ca, nil
}

func (d *kindDriver) Name() string              { return kindDriverName }
func (d *kindDriver) PlatformNamespace() string { return d.platformNS }

func (d *kindDriver) ClusterDomain(ctx context.Context) (string, error) {
	return d.clusterDomain, nil
}

func (d *kindDriver) DiscoverAPIHost(ctx context.Context) (string, error) {
	host, err := d.discoverRouteHost(ctx, "api.hypershell.localhost")
	if err != nil {
		return "", err
	}
	d.apiHost = "https://" + host
	return d.apiHost, nil
}

func (d *kindDriver) DiscoverConsoleHost(ctx context.Context) (string, error) {
	host, err := d.discoverRouteHost(ctx, "console.hypershell.localhost")
	if err != nil {
		return "", err
	}
	d.consoleHost = "https://" + host
	return d.consoleHost, nil
}

// discoverRouteHost confirms the expected HTTPRoute hostname is served by looking
// it up across HTTPRoutes, falling back to the expected hostname (the Kind overlay
// pins these hostnames) when the route is not found so a probe can report the real
// failure. A programmatic port-forward fallback is a follow-up refinement.
func (d *kindDriver) discoverRouteHost(ctx context.Context, want string) (string, error) {
	routes, err := d.clients.Gateway.GatewayV1().HTTPRoutes(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("list HTTPRoutes: %w", err)
	}
	for _, r := range routes.Items {
		for _, h := range r.Spec.Hostnames {
			if string(h) == want {
				return want, nil
			}
		}
	}
	return want, nil
}

func (d *kindDriver) DiscoverGatewayEndpoint(ctx context.Context, gw GatewayRef) (string, error) {
	route, err := d.clients.Gateway.GatewayV1().GRPCRoutes(gw.Namespace).Get(ctx, "openshell-gateway", metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get GRPCRoute openshell-gateway in %s: %w", gw.Namespace, err)
	}
	if len(route.Spec.Hostnames) == 0 {
		return "", fmt.Errorf("GRPCRoute openshell-gateway in %s has no hostnames", gw.Namespace)
	}
	host := string(route.Spec.Hostnames[0])
	return fmt.Sprintf("https://%s:443", host), nil
}

func (d *kindDriver) WaitForGatewayRoute(ctx context.Context, gw GatewayRef) error {
	timeout := durationEnv("E2E_PROVISION_TIMEOUT", 180*time.Second)
	return harness.Poll(ctx, 5*time.Second, timeout, func(ctx context.Context) (bool, error) {
		route, err := d.clients.Gateway.GatewayV1().GRPCRoutes(gw.Namespace).Get(ctx, "openshell-gateway", metav1.GetOptions{})
		if err != nil {
			return false, nil // route may not exist yet
		}
		if !grpcRouteAccepted(route) {
			return false, nil
		}
		// Confirm the parent Gateway is Programmed.
		for _, pr := range route.Spec.ParentRefs {
			ns := gw.Namespace
			if pr.Namespace != nil {
				ns = string(*pr.Namespace)
			}
			parent, err := d.clients.Gateway.GatewayV1().Gateways(ns).Get(ctx, string(pr.Name), metav1.GetOptions{})
			if err != nil {
				return false, nil
			}
			if gatewayProgrammed(parent) {
				return true, nil
			}
		}
		return false, nil
	})
}

func (d *kindDriver) APIClient(ctx context.Context, tok Token) (*apiclient.Client, error) {
	host := d.apiHost
	if host == "" {
		host = kindDefaultAPIHost
	}
	return apiclient.New(host, tok.AccessToken, d.httpClient)
}

func (d *kindDriver) DeSeedTestUsers(ctx context.Context) error { return nil }

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func durationEnv(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	// Bare integers are seconds (the Bash contract); a unit suffix is honored too.
	if secs, err := time.ParseDuration(v); err == nil {
		return secs
	}
	if n, err := time.ParseDuration(v + "s"); err == nil {
		return n
	}
	return def
}

// decodeJWTClaims returns the claim map from a JWT access token without verifying
// its signature (the suite only reads claims to check role propagation).
func decodeJWTClaims(token string) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("token is not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode JWT payload: %w", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("unmarshal JWT claims: %w", err)
	}
	return claims, nil
}

// tokenHasRole reports whether the token carries role in the nested
// hypershell.roles claim (the gateway/CLI role claim path).
func tokenHasRole(token, role string) bool {
	claims, err := decodeJWTClaims(token)
	if err != nil {
		return false
	}
	return slices.Contains(extractHypershellRoles(claims), role)
}

func extractHypershellRoles(claims map[string]any) []string {
	var out []string
	collect := func(v any) {
		if list, ok := v.([]any); ok {
			for _, item := range list {
				if s, ok := item.(string); ok {
					out = append(out, s)
				}
			}
		}
	}
	// Nested: {"hypershell": {"roles": [...]}}
	if nested, ok := claims["hypershell"].(map[string]any); ok {
		collect(nested["roles"])
	}
	// Flat: {"hypershell.roles": [...]}
	collect(claims["hypershell.roles"])
	return out
}

// escapeForm builds an application/x-www-form-urlencoded body.
func escapeForm(fields map[string]string) string {
	v := url.Values{}
	for k, val := range fields {
		v.Set(k, val)
	}
	return v.Encode()
}
