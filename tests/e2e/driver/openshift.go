package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/openshift-online/hypershell/tests/e2e/apiclient"
	"github.com/openshift-online/hypershell/tests/e2e/harness"
)

const openshiftDriverName = "openshift"

var routeGVR = schema.GroupVersionResource{Group: "route.openshift.io", Version: "v1", Resource: "routes"}

func init() {
	Register(openshiftDriverName, func(c *harness.Clients) (E2EInfraDriver, error) {
		return newOpenShiftDriver(c)
	})
}

// openshiftDriver implements E2EInfraDriver against an OpenShift cluster: hosts are
// discovered from Routes (route.openshift.io), the OIDC issuer from the Keycloak
// Route (see discoverKeycloakHost for how its namespace is resolved), and TLS uses
// the system roots (ROSA Routes serve valid certs), so there is no CA injection or
// host rewrite as on Kind. Bring-up (make openshift-up) is owned by
// openshift-development.spec.md; this is the driver the suite calls after that
// environment exists.
type openshiftDriver struct {
	clients *harness.Clients
	dyn     dynamic.Interface

	platformNS    string
	keycloakNS    string
	keycloakRoute string
	oidcIssuer    string
	frontendID    string

	kcAdminUser     string
	kcAdminPassword string

	httpClient *http.Client

	apiHost   string
	gcPatched bool
}

func newOpenShiftDriver(clients *harness.Clients) (*openshiftDriver, error) {
	dyn, err := dynamic.NewForConfig(clients.Config)
	if err != nil {
		return nil, fmt.Errorf("build dynamic client: %w", err)
	}

	platformNS := os.Getenv("OPENSHIFT_NAMESPACE")
	if platformNS == "" {
		platformNS = clients.ContextNamespace()
	}
	if platformNS == "" {
		return nil, fmt.Errorf("OPENSHIFT_NAMESPACE is unset and the KUBECONFIG context has no namespace; set OPENSHIFT_NAMESPACE to the HyperShell project")
	}

	d := &openshiftDriver{
		clients:         clients,
		dyn:             dyn,
		platformNS:      platformNS,
		keycloakRoute:   envOr("E2E_OPENSHIFT_KEYCLOAK_ROUTE", "keycloak"),
		frontendID:      envOr("E2E_OIDC_CLIENT_ID", "hypershell-frontend"),
		kcAdminUser:     envOr("E2E_KC_ADMIN_USER", "admin"),
		kcAdminPassword: envOr("E2E_KC_ADMIN_PASSWORD", "admin"),
		// ROSA Routes present valid certificates; the system roots trust them.
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}

	// Derive the OIDC issuer from the Keycloak Route, unless pinned.
	issuer := os.Getenv("E2E_OIDC_ISSUER")
	if issuer == "" {
		host, err := d.discoverKeycloakHost(context.Background())
		if err != nil {
			return nil, err
		}
		issuer = "https://" + host + "/realms/hypershell"
		// Expose it so the suite's gateway OIDC config uses the same issuer.
		_ = os.Setenv("E2E_OIDC_ISSUER", issuer)
	}
	d.oidcIssuer = issuer
	return d, nil
}

// discoverKeycloakHost returns the spec.host of the Keycloak Route, resolving its
// namespace in order of decreasing specificity so the suite works across different
// deployment layouts without code changes:
//
//  1. E2E_OPENSHIFT_KEYCLOAK_NAMESPACE, when set, is authoritative: the Route must
//     live there, and a miss is a hard error (a wrong override should fail loudly
//     rather than silently resolve to some other cluster's Keycloak).
//  2. the "<platform>-keycloak" convention the PR-environment deploys use.
//  3. a cluster-wide search for a Route named keycloakRoute (default "keycloak"),
//     so GitOps layouts that place it elsewhere (e.g. "keycloak-system") resolve
//     with no configuration.
//
// (2) falls through to (3) on a miss. The resolved namespace is recorded on the
// driver for diagnostics. E2E_OIDC_ISSUER short-circuits this entirely (handled by
// the caller).
func (d *openshiftDriver) discoverKeycloakHost(ctx context.Context) (string, error) {
	route := d.keycloakRoute

	if ns := os.Getenv("E2E_OPENSHIFT_KEYCLOAK_NAMESPACE"); ns != "" {
		host, err := d.routeHost(ctx, ns, route)
		if err != nil {
			return "", fmt.Errorf("discover Keycloak route %s/%s (E2E_OPENSHIFT_KEYCLOAK_NAMESPACE): %w", ns, route, err)
		}
		d.keycloakNS = ns
		return host, nil
	}

	conventional := d.platformNS + "-keycloak"
	if host, err := d.routeHost(ctx, conventional, route); err == nil {
		d.keycloakNS = conventional
		return host, nil
	}

	ns, host, err := d.findRouteClusterWide(ctx, route)
	if err != nil {
		return "", fmt.Errorf("discover Keycloak route %q: not found in %s and %w; set E2E_OPENSHIFT_KEYCLOAK_NAMESPACE or E2E_OIDC_ISSUER", route, conventional, err)
	}
	d.keycloakNS = ns
	return host, nil
}

// findRouteClusterWide returns the namespace and spec.host of the single Route
// named `name` across all namespaces. It requires cluster-wide list access on
// routes; it errors if none match, or if more than one does (so an ambiguous
// fleet makes the caller pin E2E_OPENSHIFT_KEYCLOAK_NAMESPACE rather than guess).
func (d *openshiftDriver) findRouteClusterWide(ctx context.Context, name string) (string, string, error) {
	list, err := d.dyn.Resource(routeGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", "", fmt.Errorf("list routes cluster-wide: %w", err)
	}
	var matches []*unstructured.Unstructured
	for i := range list.Items {
		if list.Items[i].GetName() == name {
			matches = append(matches, &list.Items[i])
		}
	}
	switch len(matches) {
	case 0:
		return "", "", fmt.Errorf("no Route named %q in any namespace", name)
	case 1:
		obj := matches[0]
		host, found, err := unstructured.NestedString(obj.Object, "spec", "host")
		if err != nil || !found || host == "" {
			return "", "", fmt.Errorf("route %s/%s has no spec.host", obj.GetNamespace(), name)
		}
		return obj.GetNamespace(), host, nil
	default:
		nss := make([]string, 0, len(matches))
		for _, m := range matches {
			nss = append(nss, m.GetNamespace())
		}
		return "", "", fmt.Errorf("found %d Routes named %q (namespaces: %s)", len(matches), name, strings.Join(nss, ", "))
	}
}

func (d *openshiftDriver) Name() string              { return openshiftDriverName }
func (d *openshiftDriver) PlatformNamespace() string { return d.platformNS }

// routeHost returns a Route's spec.host via the dynamic client.
func (d *openshiftDriver) routeHost(ctx context.Context, ns, name string) (string, error) {
	obj, err := d.dyn.Resource(routeGVR).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get route %s/%s: %w", ns, name, err)
	}
	host, found, err := unstructured.NestedString(obj.Object, "spec", "host")
	if err != nil || !found || host == "" {
		return "", fmt.Errorf("route %s/%s has no spec.host", ns, name)
	}
	return host, nil
}

func (d *openshiftDriver) DiscoverAPIHost(ctx context.Context) (string, error) {
	host, err := d.routeHost(ctx, d.platformNS, "hypershell-api")
	if err != nil {
		return "", err
	}
	d.apiHost = "https://" + host
	return d.apiHost, nil
}

func (d *openshiftDriver) DiscoverConsoleHost(ctx context.Context) (string, error) {
	host, err := d.routeHost(ctx, d.platformNS, "hypershell-web-console")
	if err != nil {
		return "", err
	}
	return "https://" + host, nil
}

func (d *openshiftDriver) DiscoverGatewayEndpoint(ctx context.Context, gw GatewayRef) (string, error) {
	route, err := d.clients.Gateway.GatewayV1().GRPCRoutes(gw.Namespace).Get(ctx, "openshell-gateway", metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get GRPCRoute openshell-gateway in %s: %w", gw.Namespace, err)
	}
	if len(route.Spec.Hostnames) == 0 {
		return "", fmt.Errorf("GRPCRoute openshell-gateway in %s has no hostnames", gw.Namespace)
	}
	return fmt.Sprintf("https://%s:443", string(route.Spec.Hostnames[0])), nil
}

// ClusterDomain returns the gateway base domain from the shared Gateway listener
// hostname (the value make openshift-up configured), overridable via
// E2E_GATEWAY_BASE_DOMAIN.
func (d *openshiftDriver) ClusterDomain(ctx context.Context) (string, error) {
	if v := os.Getenv("E2E_GATEWAY_BASE_DOMAIN"); v != "" {
		return v, nil
	}
	gws, err := d.clients.Gateway.GatewayV1().Gateways("openshift-ingress").List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("list shared gateways: %w", err)
	}
	for _, gw := range gws.Items {
		for _, l := range gw.Spec.Listeners {
			if l.Hostname != nil {
				// Strip a leading wildcard ("*.gwlb...." -> "gwlb....").
				return strings.TrimPrefix(string(*l.Hostname), "*."), nil
			}
		}
	}
	return "", fmt.Errorf("no shared Gateway listener hostname found in openshift-ingress")
}

func (d *openshiftDriver) WaitForGatewayRoute(ctx context.Context, gw GatewayRef) error {
	timeout := durationEnv("E2E_PROVISION_TIMEOUT", 180*time.Second)
	return waitForGatewayRoute(ctx, d.clients.Gateway, gw, timeout)
}

func (d *openshiftDriver) APIClient(ctx context.Context, tok Token) (*apiclient.Client, error) {
	host := d.apiHost
	if host == "" {
		h, err := d.DiscoverAPIHost(ctx)
		if err != nil {
			return nil, err
		}
		host = h
	}
	return apiclient.New(host, tok.AccessToken, d.httpClient)
}

func (d *openshiftDriver) DeSeedTestUsers(ctx context.Context) error { return nil }

// --- OIDC ---

func (d *openshiftDriver) tokenEndpoint() string {
	return strings.TrimSuffix(d.oidcIssuer, "/") + "/protocol/openid-connect/token"
}

func (d *openshiftDriver) AcquireOIDCToken(ctx context.Context, user Credentials) (Token, error) {
	grant := envOr("E2E_OIDC_GRANT", grantPassword)
	switch grant {
	case grantPassword:
		return postTokenForm(ctx, d.httpClient, d.tokenEndpoint(), map[string]string{
			"grant_type": grantPassword,
			"client_id":  d.frontendID,
			"username":   user.Username,
			"password":   user.Password,
		})
	case grantClientCredentials:
		if user.Username == "" || user.Username == envOr("E2E_OIDC_USERNAME", "admin") {
			return d.clientCredentialsToken(ctx)
		}
		return d.gatewayTokenExchange(ctx, d.frontendID, user.Username)
	default:
		return Token{}, fmt.Errorf("unsupported E2E_OIDC_GRANT %q", grant)
	}
}

func (d *openshiftDriver) AcquireClientCredentialsToken(ctx context.Context, clientID, clientSecret string) (Token, error) {
	return postTokenForm(ctx, d.httpClient, d.tokenEndpoint(), map[string]string{
		"grant_type":    grantClientCredentials,
		"client_id":     clientID,
		"client_secret": clientSecret,
	})
}

func (d *openshiftDriver) clientCredentialsToken(ctx context.Context) (Token, error) {
	saID := os.Getenv("E2E_OIDC_SA_CLIENT_ID")
	saSecret := os.Getenv("E2E_OIDC_SA_CLIENT_SECRET")
	if saID == "" || saSecret == "" {
		return Token{}, fmt.Errorf("client_credentials grant requires E2E_OIDC_SA_CLIENT_ID and E2E_OIDC_SA_CLIENT_SECRET")
	}
	return d.AcquireClientCredentialsToken(ctx, saID, saSecret)
}

func (d *openshiftDriver) AcquireGatewayTokenWithRole(ctx context.Context, user Credentials, clientID, role string) (Token, error) {
	timeout := durationEnv("E2E_GATEWAY_TOKEN_TIMEOUT", 300*time.Second)
	grant := envOr("E2E_OIDC_GRANT", grantPassword)
	var last Token
	// lastErr keeps the most recent reason the token was not usable, so a poll that
	// gives up reports why (for example a refused token exchange) instead of only
	// that the role never appeared.
	var lastErr error
	err := harness.Poll(ctx, 5*time.Second, timeout, func(ctx context.Context) (bool, error) {
		var (
			tok Token
			err error
		)
		switch grant {
		case grantPassword:
			tok, err = postTokenForm(ctx, d.httpClient, d.tokenEndpoint(), map[string]string{
				"grant_type": grantPassword,
				"client_id":  clientID,
				"username":   user.Username,
				"password":   user.Password,
			})
		case grantClientCredentials:
			tok, err = d.gatewayTokenExchange(ctx, clientID, user.Username)
		default:
			return false, fmt.Errorf("unsupported E2E_OIDC_GRANT %q", grant)
		}
		if err != nil {
			lastErr = err
			return false, nil
		}
		last = tok
		if tokenHasRole(tok.AccessToken, role) {
			return true, nil
		}
		lastErr = fmt.Errorf("token was issued but carries roles %v, not %q", tokenRoles(tok.AccessToken), role)
		return false, nil
	})
	if err != nil {
		if lastErr != nil {
			return Token{}, fmt.Errorf("gateway token for client %s never gained role %q: %w (last token error: %v)", clientID, role, err, lastErr)
		}
		return Token{}, fmt.Errorf("gateway token for client %s never gained role %q: %w", clientID, role, err)
	}
	return last, nil
}

// AcquireGatewayToken mints a per-gateway token for user without requiring any
// role (see the interface doc). It retries only transient token-endpoint errors.
func (d *openshiftDriver) AcquireGatewayToken(ctx context.Context, user Credentials, clientID string) (Token, error) {
	timeout := durationEnv("E2E_GATEWAY_TOKEN_TIMEOUT", 300*time.Second)
	grant := envOr("E2E_OIDC_GRANT", grantPassword)

	var last Token
	err := harness.Poll(ctx, 5*time.Second, timeout, func(ctx context.Context) (bool, error) {
		var (
			tok Token
			err error
		)
		switch grant {
		case grantPassword:
			tok, err = postTokenForm(ctx, d.httpClient, d.tokenEndpoint(), map[string]string{
				"grant_type": grantPassword,
				"client_id":  clientID,
				"username":   user.Username,
				"password":   user.Password,
			})
		case grantClientCredentials:
			tok, err = d.gatewayTokenExchange(ctx, clientID, user.Username)
		default:
			return false, fmt.Errorf("unsupported E2E_OIDC_GRANT %q", grant)
		}
		if err != nil {
			return false, nil // transient; keep polling until the client is ready
		}
		last = tok
		return true, nil
	})
	if err != nil {
		return Token{}, fmt.Errorf("gateway token for client %s was never issued: %w", clientID, err)
	}
	return last, nil
}

func (d *openshiftDriver) gatewayTokenExchange(ctx context.Context, clientID, requestedSubject string) (Token, error) {
	subjectTok, err := d.clientCredentialsToken(ctx)
	if err != nil {
		return Token{}, err
	}
	saID := os.Getenv("E2E_OIDC_SA_CLIENT_ID")
	saSecret := os.Getenv("E2E_OIDC_SA_CLIENT_SECRET")
	fields := map[string]string{
		"grant_type":         grantTokenExchange,
		"client_id":          saID,
		"client_secret":      saSecret,
		"subject_token":      subjectTok.AccessToken,
		"subject_token_type": tokenTypeAccessToken,
		"audience":           clientID,
	}
	if requestedSubject != "" && requestedSubject != envOr("E2E_OIDC_USERNAME", "admin") {
		fields["requested_subject"] = requestedSubject
	}
	return postTokenForm(ctx, d.httpClient, d.tokenEndpoint(), fields)
}

// --- Keycloak admin role helpers ---

func (d *openshiftDriver) kcBaseAndRealm() (base, realm string) {
	const marker = "/realms/"
	issuer := strings.TrimSuffix(d.oidcIssuer, "/")
	if idx := strings.LastIndex(issuer, marker); idx >= 0 {
		return issuer[:idx], issuer[idx+len(marker):]
	}
	return issuer, "hypershell"
}

func (d *openshiftDriver) kcAdminToken(ctx context.Context) (string, error) {
	base, _ := d.kcBaseAndRealm()
	tok, err := postTokenForm(ctx, d.httpClient, base+"/realms/master/protocol/openid-connect/token", map[string]string{
		"grant_type": grantPassword,
		"client_id":  "admin-cli",
		"username":   d.kcAdminUser,
		"password":   d.kcAdminPassword,
	})
	if err != nil {
		return "", fmt.Errorf("acquire Keycloak admin token: %w", err)
	}
	return tok.AccessToken, nil
}

func (d *openshiftDriver) kcAdminBase() string {
	base, realm := d.kcBaseAndRealm()
	return base + "/admin/realms/" + realm
}

func (d *openshiftDriver) AssignGatewayClientRole(ctx context.Context, user, clientID, role string) error {
	adminTok, err := d.kcAdminToken(ctx)
	if err != nil {
		return err
	}
	clientUUID, err := kcLookupID(ctx, d.httpClient, d.kcAdminBase()+"/clients?clientId="+url.QueryEscape(clientID), adminTok)
	if err != nil {
		return fmt.Errorf("lookup client %q: %w", clientID, err)
	}
	userUUID, err := kcLookupID(ctx, d.httpClient, d.kcAdminBase()+"/users?exact=true&username="+url.QueryEscape(user), adminTok)
	if err != nil {
		return fmt.Errorf("lookup user %q: %w", user, err)
	}
	r, err := kcGetRole(ctx, d.httpClient, d.kcAdminBase()+"/clients/"+clientUUID+"/roles/"+url.PathEscape(role), adminTok)
	if err != nil {
		return fmt.Errorf("lookup client role %q: %w", role, err)
	}
	status, body, err := kcDoJSON(ctx, d.httpClient, http.MethodPost, d.kcAdminBase()+"/users/"+userUUID+"/role-mappings/clients/"+clientUUID, adminTok, []kcRole{r})
	if err != nil {
		return err
	}
	if !okStatus(status) {
		return fmt.Errorf("assign client role %q to %q: status %d: %s", role, user, status, body)
	}
	return nil
}

func (d *openshiftDriver) AssignRealmRole(ctx context.Context, user, role string) error {
	adminTok, err := d.kcAdminToken(ctx)
	if err != nil {
		return err
	}
	userUUID, err := kcLookupID(ctx, d.httpClient, d.kcAdminBase()+"/users?exact=true&username="+url.QueryEscape(user), adminTok)
	if err != nil {
		return fmt.Errorf("lookup user %q: %w", user, err)
	}
	r, err := kcGetRole(ctx, d.httpClient, d.kcAdminBase()+"/roles/"+url.PathEscape(role), adminTok)
	if err != nil {
		return fmt.Errorf("lookup realm role %q: %w", role, err)
	}
	status, body, err := kcDoJSON(ctx, d.httpClient, http.MethodPost, d.kcAdminBase()+"/users/"+userUUID+"/role-mappings/realm", adminTok, []kcRole{r})
	if err != nil {
		return err
	}
	if !okStatus(status) {
		return fmt.Errorf("assign realm role %q to %q: status %d: %s", role, user, status, body)
	}
	return nil
}

// --- Namespace GC timing ---

func (d *openshiftDriver) ConfigureNamespaceGCTiming(ctx context.Context, interval, grace time.Duration) error {
	if err := setControllerGCEnv(ctx, d.clients, d.platformNS, map[string]string{
		gcEnvInterval:       interval.String(),
		gcEnvGrace:          grace.String(),
		directoryRefreshEnv: durationEnv("E2E_GATEWAY_DIRECTORY_REFRESH_INTERVAL", 15*time.Second).String(),
	}, nil); err != nil {
		return err
	}
	d.gcPatched = true
	return waitControllerRollout(ctx, d.clients, d.platformNS)
}

func (d *openshiftDriver) RestoreNamespaceGCTiming(ctx context.Context) error {
	if !d.gcPatched {
		return nil
	}
	if err := setControllerGCEnv(ctx, d.clients, d.platformNS, nil, []string{gcEnvInterval, gcEnvGrace, directoryRefreshEnv}); err != nil {
		return err
	}
	d.gcPatched = false
	return waitControllerRollout(ctx, d.clients, d.platformNS)
}

// --- shared Keycloak HTTP helpers (used by both drivers) ---

// postTokenForm issues a form-encoded token request and returns the access token.
func postTokenForm(ctx context.Context, client *http.Client, endpoint string, fields map[string]string) (Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(escapeForm(fields)))
	if err != nil {
		return Token{}, fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return Token{}, fmt.Errorf("token request to %s: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return Token{}, fmt.Errorf("parse token response (status %d): %w", resp.StatusCode, err)
	}
	if tr.AccessToken == "" {
		return Token{}, &tokenEndpointError{Status: resp.StatusCode, Code: tr.Error, Description: tr.ErrorDescription}
	}
	return Token{AccessToken: tr.AccessToken}, nil
}

// tokenEndpointError is a refused token request: the HTTP status and the OAuth
// error code and description the token endpoint returned. It carries nothing from
// the request, so a client secret or subject token cannot reach a test log.
type tokenEndpointError struct {
	Status      int
	Code        string
	Description string
}

func (e *tokenEndpointError) Error() string {
	switch {
	case e.Code == "" && e.Description == "":
		return fmt.Sprintf("token request failed (status %d)", e.Status)
	case e.Code == "":
		return fmt.Sprintf("token request failed (status %d): %s", e.Status, e.Description)
	case e.Description == "":
		return fmt.Sprintf("token request failed (status %d, error %s)", e.Status, e.Code)
	default:
		return fmt.Sprintf("token request failed (status %d, error %s): %s", e.Status, e.Code, e.Description)
	}
}

// tokenRoles returns the hypershell roles in an access token, or nil when the
// token is not a decodable JWT.
func tokenRoles(token string) []string {
	claims, err := decodeJWTClaims(token)
	if err != nil {
		return nil
	}
	return extractHypershellRoles(claims)
}

func kcDoJSON(ctx context.Context, client *http.Client, method, rawURL, adminTok string, body any) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return 0, nil, fmt.Errorf("marshal body: %w", err)
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, reader)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+adminTok)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	return resp.StatusCode, respBody, nil
}

// kcLookupID GETs a Keycloak collection (clients/users) and returns the first id.
func kcLookupID(ctx context.Context, client *http.Client, rawURL, adminTok string) (string, error) {
	status, body, err := kcDoJSON(ctx, client, http.MethodGet, rawURL, adminTok, nil)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("status %d: %s", status, body)
	}
	var items []kcEntity
	if err := json.Unmarshal(body, &items); err != nil {
		return "", fmt.Errorf("parse: %w", err)
	}
	if len(items) == 0 {
		return "", fmt.Errorf("no match")
	}
	return items[0].ID, nil
}

// kcGetRole GETs a single Keycloak role representation.
func kcGetRole(ctx context.Context, client *http.Client, rawURL, adminTok string) (kcRole, error) {
	status, body, err := kcDoJSON(ctx, client, http.MethodGet, rawURL, adminTok, nil)
	if err != nil {
		return kcRole{}, err
	}
	if status != http.StatusOK {
		return kcRole{}, fmt.Errorf("status %d: %s", status, body)
	}
	var r kcRole
	if err := json.Unmarshal(body, &r); err != nil {
		return kcRole{}, fmt.Errorf("parse: %w", err)
	}
	return r, nil
}

// --- shared controller GC-timing patch (used by both drivers) ---

func setControllerGCEnv(ctx context.Context, clients *harness.Clients, ns string, set map[string]string, unset []string) error {
	deploys := clients.Kube.AppsV1().Deployments(ns)
	dep, err := deploys.Get(ctx, kindControllerDeploy, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get deployment %s/%s: %w", ns, kindControllerDeploy, err)
	}
	idx := -1
	for i, c := range dep.Spec.Template.Spec.Containers {
		if c.Name == kindControllerContainer {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("deployment %s has no container %q", kindControllerDeploy, kindControllerContainer)
	}
	env := dep.Spec.Template.Spec.Containers[idx].Env
	unsetSet := make(map[string]bool, len(unset))
	for _, k := range unset {
		unsetSet[k] = true
	}
	filtered := env[:0]
	for _, e := range env {
		if unsetSet[e.Name] {
			continue
		}
		if _, replacing := set[e.Name]; replacing {
			continue
		}
		filtered = append(filtered, e)
	}
	for k, v := range set {
		filtered = append(filtered, corev1.EnvVar{Name: k, Value: v})
	}
	dep.Spec.Template.Spec.Containers[idx].Env = filtered
	if _, err := deploys.Update(ctx, dep, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update deployment %s: %w", kindControllerDeploy, err)
	}
	return nil
}

func waitControllerRollout(ctx context.Context, clients *harness.Clients, ns string) error {
	timeout := durationEnv("E2E_ROLLOUT_TIMEOUT", 300*time.Second)
	return harness.Poll(ctx, 3*time.Second, timeout, func(ctx context.Context) (bool, error) {
		dep, err := clients.Kube.AppsV1().Deployments(ns).Get(ctx, kindControllerDeploy, metav1.GetOptions{})
		if err != nil {
			return false, fmt.Errorf("get deployment %s: %w", kindControllerDeploy, err)
		}
		desired := int32(1)
		if dep.Spec.Replicas != nil {
			desired = *dep.Spec.Replicas
		}
		st := dep.Status
		return st.ObservedGeneration >= dep.Generation &&
			st.UpdatedReplicas == desired &&
			st.AvailableReplicas == desired &&
			st.UnavailableReplicas == 0, nil
	})
}
