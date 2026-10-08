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
// Route in ${OPENSHIFT_NAMESPACE}-keycloak, and TLS uses the system roots (ROSA
// Routes serve valid certs), so there is no CA injection or host rewrite as on
// Kind. Bring-up (make openshift-up) is owned by openshift-development.spec.md;
// this is the driver the suite calls after that environment exists.
type openshiftDriver struct {
	clients *harness.Clients
	dyn     dynamic.Interface

	platformNS string
	keycloakNS string
	oidcIssuer string
	frontendID string

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
		keycloakNS:      platformNS + "-keycloak",
		frontendID:      envOr("E2E_OIDC_CLIENT_ID", "hypershell-frontend"),
		kcAdminUser:     envOr("E2E_KC_ADMIN_USER", "admin"),
		kcAdminPassword: envOr("E2E_KC_ADMIN_PASSWORD", "admin"),
		// ROSA Routes present valid certificates; the system roots trust them.
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}

	// Derive the OIDC issuer from the Keycloak Route, unless pinned.
	issuer := os.Getenv("E2E_OIDC_ISSUER")
	if issuer == "" {
		host, err := d.routeHost(context.Background(), d.keycloakNS, "keycloak")
		if err != nil {
			return nil, fmt.Errorf("discover Keycloak route in %s: %w", d.keycloakNS, err)
		}
		issuer = "https://" + host + "/realms/hypershell"
		// Expose it so the suite's gateway OIDC config uses the same issuer.
		_ = os.Setenv("E2E_OIDC_ISSUER", issuer)
	}
	d.oidcIssuer = issuer
	return d, nil
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
	return harness.Poll(ctx, 5*time.Second, timeout, func(ctx context.Context) (bool, error) {
		route, err := d.clients.Gateway.GatewayV1().GRPCRoutes(gw.Namespace).Get(ctx, "openshell-gateway", metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		if !grpcRouteAccepted(route) {
			return false, nil
		}
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
		saID := os.Getenv("E2E_OIDC_SA_CLIENT_ID")
		saSecret := os.Getenv("E2E_OIDC_SA_CLIENT_SECRET")
		if saID == "" || saSecret == "" {
			return Token{}, fmt.Errorf("client_credentials grant requires E2E_OIDC_SA_CLIENT_ID and E2E_OIDC_SA_CLIENT_SECRET")
		}
		return postTokenForm(ctx, d.httpClient, d.tokenEndpoint(), map[string]string{
			"grant_type":    grantClientCredentials,
			"client_id":     saID,
			"client_secret": saSecret,
		})
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

func (d *openshiftDriver) AcquireGatewayTokenWithRole(ctx context.Context, user Credentials, clientID, role string) (Token, error) {
	timeout := durationEnv("E2E_GATEWAY_TOKEN_TIMEOUT", 300*time.Second)
	var last Token
	err := harness.Poll(ctx, 5*time.Second, timeout, func(ctx context.Context) (bool, error) {
		tok, err := postTokenForm(ctx, d.httpClient, d.tokenEndpoint(), map[string]string{
			"grant_type": grantPassword,
			"client_id":  clientID,
			"username":   user.Username,
			"password":   user.Password,
		})
		if err != nil {
			return false, nil
		}
		last = tok
		return tokenHasRole(tok.AccessToken, role), nil
	})
	if err != nil {
		return Token{}, fmt.Errorf("gateway token for client %s never gained role %q: %w", clientID, role, err)
	}
	return last, nil
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
		gcEnvInterval: interval.String(),
		gcEnvGrace:    grace.String(),
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
	if err := setControllerGCEnv(ctx, d.clients, d.platformNS, nil, []string{gcEnvInterval, gcEnvGrace}); err != nil {
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
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return Token{}, fmt.Errorf("parse token response (status %d): %w", resp.StatusCode, err)
	}
	if tr.AccessToken == "" {
		msg := tr.ErrorDescription
		if msg == "" {
			msg = tr.Error
		}
		return Token{}, fmt.Errorf("token request failed (status %d): %s", resp.StatusCode, msg)
	}
	return Token{AccessToken: tr.AccessToken}, nil
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
	defer resp.Body.Close()
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
