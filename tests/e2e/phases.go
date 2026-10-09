package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	sdktypes "github.com/openshift-online/hypershell/components/sdk-go/types"
	"github.com/openshift-online/hypershell/tests/e2e/apiclient"
	"github.com/openshift-online/hypershell/tests/e2e/driver"
	"github.com/openshift-online/hypershell/tests/e2e/harness"
)

// discoverSeedIDs resolves the seeded managed-cluster id the suite provisions
// gateways against, selecting by E2E_SEED_CLUSTER_NAME (driver-specific default)
// and requiring a registered cluster (non-empty oidc_subject).
func (s *E2ESuite) discoverSeedIDs(ctx context.Context) (clusterID string, err error) {
	clusterName := envOrDefault("E2E_SEED_CLUSTER_NAME", s.seedDefaults())

	// The matrix runner pins the cluster id directly.
	if s.clusterIDOverride != "" {
		clusterID = s.clusterIDOverride
	}

	clusters, err := s.admin.ManagedClusters().List(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("list managed clusters: %w", err)
	}
	for _, c := range clusters.Items {
		if clusterID != "" {
			break // pinned by the matrix runner
		}
		if c.OidcSubject == "" {
			continue // not registered
		}
		if c.Name == clusterName {
			clusterID = c.ID
			break
		}
		if clusterID == "" {
			clusterID = c.ID // first registered as fallback
		}
	}
	if clusterID == "" {
		return "", fmt.Errorf("no registered managed cluster found (wanted %q; re-seed with `make %s-seed`)", clusterName, s.driver.Name())
	}
	return clusterID, nil
}

// --- Phase P0: preflight gate ---

// p0_1Authentication asserts the OIDC-authenticated API path: an unauthenticated
// call is 401, the admin token is accepted, the BFF OIDC endpoints honor their
// contract, and the control plane has no Unauthenticated gRPC errors.
func (s *E2ESuite) p0_1Authentication(t *testing.T) {
	// Unauthenticated API request is rejected with 401.
	s.runner.Comment("curl -s -o /dev/null -w '%%{http_code}' %s/api/hypershell/v1/gateways  # expect 401", s.apiHost)
	status, _, err := s.rawGet(s.apiHost+"/api/hypershell/v1/gateways", "")
	s.Require().NoError(err, "unauthenticated API probe")
	s.Assert().Equal(http.StatusUnauthorized, status, "unauthenticated API call must be 401")

	// Authenticated API request is accepted.
	s.runner.Show("GET %s/api/hypershell/v1/gateways  # with admin bearer", s.apiHost)
	_, err = s.admin.Gateways().List(t.Context(), nil)
	s.Require().NoError(err, "authenticated API call must succeed")

	// BFF OIDC endpoints.
	consoleHost, err := s.driver.DiscoverConsoleHost(t.Context())
	if err != nil {
		t.Logf("WARN discover console host (skipping BFF checks): %v", err)
	} else {
		s.assertBFFSession(consoleHost)
		s.assertBFFLogin(t, consoleHost)
	}

	// Control plane gRPC watch has no Unauthenticated errors.
	s.assertNoUnauthenticatedControllerLogs(t)
}

func (s *E2ESuite) assertBFFSession(consoleHost string) {
	s.runner.Show("GET %s/auth/session  # expect {\"authenticated\":false}", consoleHost)
	status, body, err := s.rawGet(consoleHost+"/auth/session", "")
	if !s.Assert().NoError(err, "BFF /auth/session") {
		return
	}
	s.Assert().Equal(http.StatusOK, status, "BFF /auth/session status")
	s.Assert().Contains(string(body), "\"authenticated\":false", "BFF /auth/session body")
}

func (s *E2ESuite) assertBFFLogin(t *testing.T, consoleHost string) {
	s.runner.Show("GET %s/auth/login  # expect 302 to Keycloak with PKCE", consoleHost)
	client := &http.Client{
		Transport:     http.DefaultTransport,
		Timeout:       30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, consoleHost+"/auth/login", nil)
	s.Require().NoError(err)
	resp, err := client.Do(req)
	if !s.Assert().NoError(err, "BFF /auth/login") {
		return
	}
	defer func() { _ = resp.Body.Close() }()
	s.Assert().Equal(http.StatusFound, resp.StatusCode, "BFF /auth/login must 302")
	s.Assert().Contains(resp.Header.Get("Location"), "code_challenge_method=S256", "PKCE challenge method in redirect")
}

func (s *E2ESuite) assertNoUnauthenticatedControllerLogs(t *testing.T) {
	// Scope to logs produced since the run started: a long-idle cluster accumulates
	// benign token-expiry/recovery warnings that are not a failure of the watch
	// under test.
	logs, err := s.podLogsSince(t.Context(), s.driver.PlatformNamespace(), "app=hypershell-controller", s.startedAt)
	if err != nil {
		t.Logf("WARN read controller logs: %v", err)
		return
	}
	s.Assert().NotContains(logs, "Unauthenticated", "control plane gRPC watch must have no Unauthenticated errors since the run started")
}

// p0_2EnvironmentReadiness asserts the platform dependencies are healthy and the
// seed ids resolved (done in SetupSuite).
func (s *E2ESuite) p0_2EnvironmentReadiness(t *testing.T) {
	ctx := t.Context()

	// Platform dependencies are verified by their served API groups rather than
	// namespace-pinned deployments, so the check is portable across Kind and
	// OpenShift (which place these operators in different namespaces). Keycloak
	// health is already proven by P0.1 having acquired a token against it.
	for _, group := range []string{"gateway.networking.k8s.io", "cert-manager.io", "agents.x-k8s.io"} {
		served, err := s.clients.ServesAPIGroup(group)
		s.Require().NoErrorf(err, "query API group %s", group)
		s.Assert().Truef(served, "platform dependency API group %s must be served", group)
	}

	nps, err := s.clients.Kube.NetworkingV1().NetworkPolicies(s.driver.PlatformNamespace()).List(ctx, metav1.ListOptions{})
	s.Require().NoError(err, "list network policies")
	s.Assert().GreaterOrEqual(len(nps.Items), 1, "platform namespace should have baseline NetworkPolicies")

	s.Assert().NotEmpty(s.clusterID, "seeded cluster id")
}

// --- Phase P1: primary gateway up and reachable ---

func (s *E2ESuite) p1_1Provisioning(t *testing.T) {
	name := fmt.Sprintf("%s-%s", envOrDefault("E2E_GATEWAY_NAME", "e2e-gw"), s.runID)
	created := s.createGateway(t, name)
	s.trackGateway(created.ID)

	s.primary = driver.GatewayRef{Name: created.Name, Namespace: created.Namespace, ID: created.ID}

	running := s.waitGatewayRunning(t, created.ID)
	s.primary.Namespace = running.Namespace
	s.Require().Equal("Running", running.Phase, "gateway must reach Running")
	t.Logf("primary gateway %s (%s) Running in namespace %s", running.Name, running.ID, running.Namespace)
	s.startOrphanReaper(t)
}

func (s *E2ESuite) p1_2InfraVerification(t *testing.T) {
	ctx := t.Context()
	ns := s.primary.Namespace
	s.Require().NotEmpty(ns, "primary gateway namespace")

	// TLS is provisioned by cert-manager Certificates (openshell-gateway-server),
	// not a certgen Job; the populated server-TLS secret is the Certificate's
	// output and the evidence the issuance succeeded. (Divergence from the spec's
	// "certgen job" wording, which predates the cert-manager implementation.)
	var readyReplicas int32
	var clusterIP string
	var tlsCert, tlsKey []byte
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(s.concurrency)
	g.Go(func() error {
		dep, err := s.clients.Kube.AppsV1().Deployments(ns).Get(ctx, "openshell-gateway", metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("get gateway deployment: %w", err)
		}
		readyReplicas = dep.Status.ReadyReplicas
		return nil
	})
	g.Go(func() error {
		svc, err := s.clients.Kube.CoreV1().Services(ns).Get(ctx, "openshell-gateway", metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("get gateway service: %w", err)
		}
		clusterIP = svc.Spec.ClusterIP
		return nil
	})
	g.Go(func() error {
		secret, err := s.clients.Kube.CoreV1().Secrets(ns).Get(ctx, "openshell-server-tls", metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("get gateway TLS secret openshell-server-tls: %w", err)
		}
		tlsCert = secret.Data["tls.crt"]
		tlsKey = secret.Data["tls.key"]
		return nil
	})
	s.Require().NoError(g.Wait(), "parallel gateway infrastructure verification")
	s.Assert().GreaterOrEqual(int(readyReplicas), 1, "gateway deployment ready replicas")
	s.Assert().NotEmpty(clusterIP, "gateway service ClusterIP")
	s.Assert().NotEmpty(tlsCert, "server TLS cert populated")
	s.Assert().NotEmpty(tlsKey, "server TLS key populated")
}

func (s *E2ESuite) p1_3TokenAndCATrust(t *testing.T) {
	// The per-gateway Keycloak client is "<gw-name>-<gw-id>"; block until the
	// admin role lands in a token for it (roles reconcile asynchronously).
	clientID := fmt.Sprintf("%s-%s", s.primary.Name, s.primary.ID)
	s.runner.Comment("acquire per-gateway token for client %s with role openshell-admin", clientID)
	tok, err := s.driver.AcquireGatewayTokenWithRole(t.Context(), s.adminCreds(), clientID, "openshell-admin")
	s.Require().NoError(err, "acquire gateway token with openshell-admin role")
	s.Require().NotEmpty(tok.AccessToken, "gateway token")
	s.primaryGatewayToken = tok
	// CA trust is already installed process-wide for the SDK (no insecure bypass);
	// the CLI consumes the same CA via SSL_CERT_FILE in P1.4/P1.5.
}

// p1_4RouteAndCLIRegistration waits for the gateway route, discovers the endpoint,
// and registers the gateway with the openshell CLI (writes its per-gateway config).
func (s *E2ESuite) p1_4RouteAndCLIRegistration(t *testing.T) {
	s.Require().NoError(s.driver.WaitForGatewayRoute(t.Context(), s.primary), "wait for gateway route")
	endpoint, err := s.driver.DiscoverGatewayEndpoint(t.Context(), s.primary)
	s.Require().NoError(err, "discover gateway endpoint")
	t.Logf("gateway endpoint: %s", endpoint)

	s.primaryLocalName = s.registerGateway(t, s.primary, endpoint, s.primaryGatewayToken.AccessToken)
}

// p1_5Connectivity verifies the openshell CLI connects and reports status over the
// trusted TLS established in P1.3.
func (s *E2ESuite) p1_5Connectivity(t *testing.T) {
	s.Require().NotEmpty(s.primaryLocalName, "primary gateway registered with the CLI")
	s.cliStatusConnected(t, s.primaryLocalName, durationSecondsEnv("E2E_CONNECT_TIMEOUT", 90*time.Second))
}

// --- Phase P2 / P3 ---

// p2_1SandboxLifecycle creates a sandbox via the CLI, waits for its pod to run,
// execs into it, deletes it, and verifies the gateway's active_sandbox_count tracks
// create/delete. Owns its own gateway so the count assertion is isolated.
func (s *E2ESuite) p2_1SandboxLifecycle(t *testing.T) {
	ctx := t.Context()

	gw := s.createGateway(t, "e2e-sbx-"+s.runID)
	s.trackGateway(gw.ID)
	running := s.waitGatewayRunning(t, gw.ID)
	ref := driver.GatewayRef{Name: running.Name, Namespace: running.Namespace, ID: running.ID}
	s.Require().NoError(s.driver.WaitForGatewayRoute(ctx, ref), "wait for sandbox gateway route")
	endpoint, err := s.driver.DiscoverGatewayEndpoint(ctx, ref)
	s.Require().NoError(err, "discover sandbox gateway endpoint")

	clientID := fmt.Sprintf("%s-%s", ref.Name, ref.ID)
	tok, err := s.driver.AcquireGatewayTokenWithRole(ctx, s.adminCreds(), clientID, "openshell-admin")
	s.Require().NoError(err, "sandbox gateway admin token")
	local := s.registerGateway(t, ref, endpoint, tok.AccessToken)
	s.cliStatusConnected(t, local, durationSecondsEnv("E2E_CONNECT_TIMEOUT", 90*time.Second))

	sandboxTimeout := durationSecondsEnv("E2E_SANDBOX_TIMEOUT", 120*time.Second)
	// Sandbox names have a 19-char maximum, so use a short unique token rather than
	// the full run id (which carries a cluster suffix under the matrix runner).
	name := fmt.Sprintf("sb-%06d", time.Now().UnixNano()%1000000)
	// `sandbox create` streams while the sandbox image pulls and can return a stream
	// error ("missing grpc-status") even though the sandbox was created. Mirror the
	// Bash suite: treat create as best-effort and rely on the pod reaching Running
	// as the success signal rather than the command's exit status.
	if out, err := s.cli(t, local, "sandbox", "create", "--name", name); err != nil {
		t.Logf("sandbox create returned an error (continuing to poll the pod): %v\n%s", err, out)
	}
	s.Require().NoError(s.waitPodRunning(ctx, running.Namespace, "default--"+name, sandboxTimeout), "sandbox pod Running")

	// The pod can be Running while the Sandbox CR is still provisioning, so poll a
	// no-op exec for readiness before the real exec.
	s.Require().NoError(pollNoCtx(sandboxTimeout, 5*time.Second, func() bool {
		_, err := s.cli(t, local, "sandbox", "exec", "-n", name, "--", "true")
		return err == nil
	}), "sandbox exec readiness")

	out, err := s.cli(t, local, "sandbox", "exec", "-n", name, "--", "uname", "-a")
	s.Require().NoError(err, "sandbox exec uname -a")
	s.Assert().NotEmpty(strings.TrimSpace(stripANSI(out)), "uname output")

	// active_sandbox_count tracks the created sandbox, then the delete.
	s.pollActiveSandboxCount(t, gw.ID, 1)

	// delete can hit a transient "upstream request timeout"; retry, then rely on
	// the count returning to 0 as the authoritative signal.
	if err := pollNoCtx(60*time.Second, 10*time.Second, func() bool {
		out, err := s.cli(t, local, "sandbox", "delete", name)
		if err != nil {
			t.Logf("sandbox delete retrying after: %v\n%s", err, out)
		}
		return err == nil
	}); err != nil {
		t.Logf("sandbox delete did not report success; verifying via active_sandbox_count")
	}
	s.pollActiveSandboxCount(t, gw.ID, 0)
}

// p2_2RBACEnforcement checks the developer vs platform-admin boundary at the API:
// the developer may list gateways; gateway-create depends on the deployment's
// RBAC default (gateway:creator grants create, otherwise 403); platform-admin
// holds the elevated permissions including gateway delete. The developer
// sandbox-create positive case needs the CLI and lands with the CLI wave.
func (s *E2ESuite) p2_2RBACEnforcement(t *testing.T) {
	if usesSingleServiceAccountIdentity() {
		t.Skip("developer/platform-admin RBAC boundary requires distinct password-grant test users; client_credentials supplies one service-account identity")
	}

	ctx := t.Context()
	devAPI := s.apiClientForCreds(t, s.devCreds())

	// Developer may list gateways.
	st, body, err := devAPI.RawJSON(ctx, http.MethodGet, "/gateways", nil)
	s.Require().NoError(err, "developer GET /gateways")
	s.Assert().Equalf(http.StatusOK, st, "developer must list gateways (body: %s)", string(body))

	// Developer gateway-create: environment-dependent on the RBAC default.
	createBody := gatewayCreateRequestForDriver("e2e-dev-create-"+s.runID, s.driver.Name(), s.gatewayOIDCConfig())
	st, body, err = devAPI.RawJSON(ctx, http.MethodPost, "/gateways", createBody)
	s.Require().NoError(err, "developer POST /gateways")
	if s.rbacDefaultIncludesCreator(t) {
		s.Assert().Truef(st == http.StatusCreated || st == http.StatusOK, "with gateway:creator default, developer create should succeed, got %d (%s)", st, string(body))
		var created sdktypes.Gateway
		if json.Unmarshal(body, &created) == nil && created.ID != "" {
			_ = s.admin.Gateways().Delete(ctx, created.ID) // clean up
		}
	} else {
		s.Assert().Equalf(http.StatusForbidden, st, "without gateway:creator default, developer create must be 403 (body: %s)", string(body))
	}

	// Platform-admin holds elevated permissions, including gateway delete.
	s.Require().NoError(s.driver.AssignRealmRole(ctx, s.platformAdminCreds().Username, "platform:admin"), "grant platform:admin")
	padminAPI := s.apiClientForCreds(t, s.platformAdminCreds())
	st, body, err = padminAPI.RawJSON(ctx, http.MethodGet, "/gateways", nil)
	s.Require().NoError(err, "platform-admin GET /gateways")
	s.Assert().Equalf(http.StatusOK, st, "platform-admin must list gateways (body: %s)", string(body))

	victim := s.createGateway(t, "e2e-padmin-del-"+s.runID)
	st, body, err = padminAPI.RawJSON(ctx, http.MethodDelete, "/gateways/"+victim.ID, nil)
	s.Require().NoError(err, "platform-admin DELETE gateway")
	s.Assert().Truef(st == http.StatusNoContent || st == http.StatusOK || st == http.StatusAccepted, "platform-admin gateway delete should succeed, got %d (%s)", st, string(body))
}

// p2_3DeletionAndGC validates both namespace-GC paths: deleting a gateway reaps
// its managed namespace, and the periodic reaper deletes a synthetic orphan
// namespace and emits a GarbageCollected Event. The orphan path is long-only and
// relies on the shortened GC timing SetupSuite applied.
func (s *E2ESuite) p2_3DeletionAndGC(t *testing.T) {
	ctx := t.Context()

	// Delete-driven reap: create an own gateway, delete it, assert its namespace GCs.
	gw := s.createGateway(t, "e2e-del-"+s.runID)
	ns := s.waitGatewayNamespace(t, gw.ID)
	s.Require().NotEmpty(ns, "gateway namespace")
	s.runner.Show("DELETE /gateways/%s  # then expect namespace %s to be garbage-collected", gw.ID, ns)
	s.Require().NoError(s.admin.Gateways().Delete(ctx, gw.ID), "delete gateway")
	s.Require().NoError(s.waitNamespaceGone(ctx, ns, durationSecondsEnv("E2E_GC_TIMEOUT", 180*time.Second)), "managed namespace garbage-collected after delete")

	if s.mode != modeLong {
		return
	}

	// Periodic-reaper path: P1.1 seeded the orphan and started this wait in the
	// background, overlapping it with P1.2-P1.5 and the P2 fan-out.
	s.Require().NotNil(s.orphan, "background orphan reaper was started")
	select {
	case err := <-s.orphan.done:
		s.Require().NoError(err, "orphan namespace reaped by the periodic GC")
	case <-ctx.Done():
		s.Require().NoError(ctx.Err(), "wait for background orphan reaper")
	}
	s.assertGarbageCollectedEvent(t, s.orphan.name)
}

// p2_4ManagedClusterLifecycle validates ManagedCluster registration and the
// control-plane identity rules via the REST API: the seeded cluster is registered,
// /registration is idempotent for the registrar and rejects non-registrars (403)
// and name collisions (409), and gateway-create rejects an invalid placement (400).
// The gRPC WatchGateways identity checks and controller reconnect convergence are
// follow-ups.
func (s *E2ESuite) p2_4ManagedClusterLifecycle(t *testing.T) {
	ctx := t.Context()
	defClusterName := s.seedDefaults()
	clusterName := envOrDefault("E2E_SEED_CLUSTER_NAME", defClusterName)

	// 12a: the seeded cluster is registered (non-empty oidc_subject, fresh last_seen).
	seeded, err := s.admin.ManagedClusters().Get(ctx, s.clusterID)
	s.Require().NoError(err, "get seeded managed cluster")
	s.Assert().NotEmpty(seeded.OidcSubject, "registered cluster has oidc_subject")
	s.Assert().Equal(clusterName, seeded.Name, "seeded cluster name")

	// Registrar identity (client-credentials for the control-plane client).
	regTok, err := s.driver.AcquireClientCredentialsToken(ctx,
		envOrDefault("E2E_REGISTRAR_CLIENT_ID", "hypershell-control-plane"),
		envOrDefault("E2E_REGISTRAR_CLIENT_SECRET", "control-plane-secret"))
	s.Require().NoError(err, "acquire registrar client-credentials token")
	regAPI, err := s.driver.APIClient(ctx, regTok)
	s.Require().NoError(err, "registrar API client")

	// 12b: registration is idempotent and returns the same cluster_id.
	for i := range 2 {
		st, body, err := regAPI.RawJSON(ctx, http.MethodPost, "/managed_clusters/registration", map[string]string{"name": clusterName})
		s.Require().NoErrorf(err, "registration call %d", i+1)
		s.Require().Equalf(http.StatusOK, st, "registration call %d must be 200 (body: %s)", i+1, string(body))
		var reg struct {
			ClusterID string `json:"cluster_id"`
		}
		s.Require().NoError(json.Unmarshal(body, &reg), "parse registration response")
		s.Assert().Equal(s.clusterID, reg.ClusterID, "registration returns the seeded cluster id")
	}

	// 12c: a non-registrar (admin) is forbidden from registering.
	st, body, err := s.admin.RawJSON(ctx, http.MethodPost, "/managed_clusters/registration", map[string]string{"name": clusterName})
	s.Require().NoError(err, "admin registration attempt")
	s.Assert().Equalf(http.StatusForbidden, st, "non-registrar registration must be 403 (body: %s)", string(body))

	// 12d: the registrar registering a different name collides (409).
	st, body, err = regAPI.RawJSON(ctx, http.MethodPost, "/managed_clusters/registration", map[string]string{"name": "e2e-wrong-name-" + s.runID})
	s.Require().NoError(err, "registrar name-collision attempt")
	s.Assert().Equalf(http.StatusConflict, st, "registrar name collision must be 409 (body: %s)", string(body))

	// 12e: placement, rather than a caller-supplied cluster id, selects the
	// registered control plane. Unsupported placement requests are rejected.
	invalidPlacement := gatewayCreateRequestForDriver("e2e-invalid-placement-"+s.runID, s.driver.Name(), s.gatewayOIDCConfig())
	invalidPlacement.Placement = gatewayPlacementIntent{Network: "invalid", Provider: "aws"}
	st, body, err = s.admin.RawJSON(ctx, http.MethodPost, "/gateways", invalidPlacement)
	s.Require().NoError(err, "gateway create with invalid placement")
	s.Assert().Equalf(http.StatusBadRequest, st, "invalid placement must be 400 (body: %s)", string(body))
	s.Assert().Containsf(strings.ToLower(string(body)), "placement", "400 reason should name placement (body: %s)", string(body))

	t.Log("NOTE: gRPC WatchGateways identity rejection and controller reconnect convergence are follow-ups")
}

// p3_1AdminInventory verifies the admin-only /users boundary (non-admin 403,
// opaque 404 on a singleton Get) and rejection of an unknown gateway phase write
// by the primary gateway's owner.
// Password-grant runs compare the developer and platform-admin users; brokered
// client-credentials runs compare the privileged hypershell-e2e service account
// and the unprivileged hypershell-control-plane service account.
func (s *E2ESuite) p3_1AdminInventory(t *testing.T) {
	ctx := t.Context()

	privilegedAPI := s.admin
	privilegedLabel := "platform-admin"
	// The primary gateway's creator owns it. Its owner binding authorizes PATCH,
	// while platform:admin is intentionally inventory-only for gateway mutation.
	phaseValidationAPI := s.admin
	var unprivilegedAPI *apiclient.Client
	unprivilegedLabel := "developer"
	if usesSingleServiceAccountIdentity() {
		privilegedLabel = "hypershell-e2e service account"
		unprivilegedLabel = "hypershell-control-plane service account"
		tok, err := s.driver.AcquireClientCredentialsToken(ctx,
			envOrDefault("E2E_REGISTRAR_CLIENT_ID", "hypershell-control-plane"),
			envOrDefault("E2E_REGISTRAR_CLIENT_SECRET", "control-plane-secret"))
		s.Require().NoError(err, "acquire unprivileged control-plane client-credentials token")
		unprivilegedAPI, err = s.driver.APIClient(ctx, tok)
		s.Require().NoError(err, "build unprivileged control-plane API client")
	} else {
		unprivilegedAPI = s.apiClientForCreds(t, s.devCreds())
		s.Require().NoError(s.driver.AssignRealmRole(ctx, s.platformAdminCreds().Username, "platform:admin"), "grant platform:admin")
		privilegedAPI = s.apiClientForCreds(t, s.platformAdminCreds())
	}

	s.runner.Show("GET /users  # as %s, expect 403", unprivilegedLabel)
	st, body, err := unprivilegedAPI.RawJSON(ctx, http.MethodGet, "/users", nil)
	s.Require().NoErrorf(err, "%s GET /users", unprivilegedLabel)
	s.Assert().Equalf(http.StatusForbidden, st, "%s GET /users must be 403 (body: %s)", unprivilegedLabel, string(body))

	s.runner.Show("GET /users?size=1  # as %s, expect 200", privilegedLabel)
	st, body, err = privilegedAPI.RawJSON(ctx, http.MethodGet, "/users?size=1", nil)
	s.Require().NoErrorf(err, "%s GET /users", privilegedLabel)
	s.Require().Equalf(http.StatusOK, st, "%s GET /users must be 200 (body: %s)", privilegedLabel, string(body))
	var userList struct {
		Total int `json:"total"`
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	s.Require().NoError(json.Unmarshal(body, &userList), "parse user list")
	s.Assert().GreaterOrEqual(userList.Total, 1, "user list total")

	if len(userList.Items) > 0 {
		id := userList.Items[0].ID
		s.runner.Show("GET /users/%s  # as %s, expect opaque 404", id, unprivilegedLabel)
		st, _, err = unprivilegedAPI.RawJSON(ctx, http.MethodGet, "/users/"+id, nil)
		s.Require().NoErrorf(err, "%s GET /users/{id}", unprivilegedLabel)
		s.Assert().Equalf(http.StatusNotFound, st, "%s singleton user Get must be opaque 404", unprivilegedLabel)
	}

	s.runner.Show("PATCH /gateways/%s {\"phase\":\"Bogus-e2e-phase\"}  # as gateway owner, expect 400", s.primary.ID)
	st, body, err = phaseValidationAPI.RawJSON(ctx, http.MethodPatch, "/gateways/"+s.primary.ID, map[string]string{"phase": "Bogus-e2e-phase"})
	s.Require().NoErrorf(err, "gateway owner patch gateway phase")
	s.Assert().Equalf(http.StatusBadRequest, st, "unknown phase write must be 400 (body: %s)", string(body))
	s.Assert().Containsf(strings.ToLower(string(body)), "phase", "400 reason should name the phase field (body: %s)", string(body))
}

// --- low-level helpers ---

// gatewayPlacementIntent is the client-selected placement intent. The API server
// resolves it to a registered managed-cluster id during gateway creation.
type gatewayPlacementIntent struct {
	Mode     string `json:"mode,omitempty"`
	Network  string `json:"network,omitempty"`
	Provider string `json:"provider,omitempty"`
}

// gatewayCreateRequest contains only fields accepted by GatewayCreateRequest.
// In particular, cluster_id is server-derived from Placement and must not be
// sent by callers.
type gatewayCreateRequest struct {
	Name      string                 `json:"name"`
	Placement gatewayPlacementIntent `json:"placement"`
	Route     string                 `json:"route"`
	OIDC      string                 `json:"oidc,omitempty"`
}

// gatewayCreateRequestForDriver builds the infra-specific create payload used by
// functional and performance e2e tests. Kind resolves the local development
// control plane; OpenShift resolves an available public AWS control plane.
func gatewayCreateRequestForDriver(name, driverName, oidc string) gatewayCreateRequest {
	placement := gatewayPlacementIntent{Network: "public", Provider: "aws"}
	if driverName == "kind" {
		placement = gatewayPlacementIntent{Mode: "local-kind"}
	}
	return gatewayCreateRequest{
		Name:      name,
		Placement: placement,
		Route:     `{"enabled":true}`,
		OIDC:      oidc,
	}
}

// createGateway provisions a gateway via the create endpoint. Route and OIDC are
// JSON-encoded string fields (matching the seed and the API schema):
// route.enabled=true makes the control plane create the gateway's GRPCRoute, and
// the oidc config (issuer/audience) lets the gateway validate the CLI's bearer
// token. namespace is server-derived and must not be sent. It uses the raw helper
// rather than the SDK's Create, which also serializes server-managed fields (for
// example the readOnly namespace) that the strict create endpoint rejects.
func (s *E2ESuite) createGateway(t *testing.T, name string) *sdktypes.Gateway {
	body := gatewayCreateRequestForDriver(name, s.driver.Name(), s.gatewayOIDCConfig())
	s.runner.Show("POST %s/api/hypershell/v1/gateways  # name=%s placement=%+v", s.apiHost, name, body.Placement)
	status, resp, err := s.admin.RawJSON(t.Context(), http.MethodPost, "/gateways", body)
	s.Require().NoError(err, "create gateway request")
	s.Require().Equalf(http.StatusCreated, status, "create gateway status (body: %s)", string(resp))

	var created sdktypes.Gateway
	s.Require().NoError(json.Unmarshal(resp, &created), "unmarshal created gateway")
	s.Require().NotEmpty(created.ID, "created gateway id")
	return &created
}

// gatewayOIDCConfig returns the JSON-encoded gateway OIDC config string. The
// issuer and audience match what the driver uses to mint tokens (kind defaults,
// overridable via env); the openshift driver sets these for its environment.
func (s *E2ESuite) gatewayOIDCConfig() string {
	issuer := envOrDefault("E2E_OIDC_ISSUER", "https://keycloak.hypershell.localhost/realms/hypershell")
	audience := envOrDefault("E2E_OIDC_CLIENT_ID", "hypershell-frontend")
	return fmt.Sprintf(`{"issuer":%q,"audience":%q,"roles_claim":"groups","admin_role":"hypershell-admins","user_role":"hypershell-users"}`, issuer, audience)
}

// apiClientForCreds acquires an OIDC token for creds and returns an API client
// carrying it.
func (s *E2ESuite) apiClientForCreds(t *testing.T, creds driver.Credentials) *apiclient.Client {
	tok, err := s.driver.AcquireOIDCToken(t.Context(), creds)
	s.Require().NoErrorf(err, "acquire token for %s", creds.Username)
	api, err := s.driver.APIClient(t.Context(), tok)
	s.Require().NoErrorf(err, "build API client for %s", creds.Username)
	return api
}

// waitGatewayRunning polls the gateway until it reaches Running or the provision
// timeout elapses, returning the last observed gateway.
func (s *E2ESuite) waitGatewayRunning(t *testing.T, id string) *sdktypes.Gateway {
	timeout := durationSecondsEnv("E2E_PROVISION_TIMEOUT", 180*time.Second)
	var last *sdktypes.Gateway
	err := harness.Poll(t.Context(), 5*time.Second, timeout, func(ctx context.Context) (bool, error) {
		gw, err := s.admin.Gateways().Get(ctx, id)
		if err != nil {
			return false, nil // transient
		}
		last = gw
		return gw.Phase == "Running", nil
	})
	if err != nil {
		s.dumpGatewayDiagnostics(t, id, last)
		phase := "<unknown>"
		if last != nil {
			phase = last.Phase
		}
		s.Require().NoError(err, "gateway %s never reached Running (last phase %s)", id, phase)
	}
	return last
}

// waitGatewayNamespace waits only until reconciliation has assigned and created
// the managed namespace. Deletion/GC coverage does not need to pay for the rest
// of gateway provisioning before it can exercise namespace finalization.
func (s *E2ESuite) waitGatewayNamespace(t *testing.T, id string) string {
	timeout := durationSecondsEnv("E2E_PROVISION_TIMEOUT", 180*time.Second)
	var namespace string
	err := harness.Poll(t.Context(), 3*time.Second, timeout, func(ctx context.Context) (bool, error) {
		gw, err := s.admin.Gateways().Get(ctx, id)
		if err != nil || gw.Namespace == "" {
			return false, nil
		}
		namespace = gw.Namespace
		_, err = s.clients.Kube.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		return true, nil
	})
	s.Require().NoError(err, "gateway %s never received a managed namespace", id)
	return namespace
}

// dumpGatewayDiagnostics logs the gateway's phase and provisioning conditions plus
// recent controller logs, so a provisioning timeout is debuggable from the run.
func (s *E2ESuite) dumpGatewayDiagnostics(t *testing.T, id string, gw *sdktypes.Gateway) {
	if gw != nil {
		t.Logf("DIAG gateway %s phase=%q namespace=%q conditions=%v", id, gw.Phase, gw.Namespace, gw.ProvisioningConditions)
	}
	if logs, err := s.podLogs(context.Background(), s.driver.PlatformNamespace(), "app=hypershell-controller", 40); err == nil {
		t.Logf("DIAG controller logs (tail):\n%s", logs)
	}
}

// rawGet issues an unauthenticated-or-bearer GET through the CA-trusting default
// transport and returns the status and body.
func (s *E2ESuite) rawGet(url, token string) (int, []byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return 0, nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, body, nil
}

// podLogsSince returns logs from the first pod matching selector in ns produced
// since the given time.
func (s *E2ESuite) podLogsSince(ctx context.Context, ns, selector string, since time.Time) (string, error) {
	pods, err := s.clients.Kube.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return "", fmt.Errorf("list pods %s in %s: %w", selector, ns, err)
	}
	if len(pods.Items) == 0 {
		return "", fmt.Errorf("no pods match %s in %s", selector, ns)
	}
	sinceTime := metav1.NewTime(since)
	req := s.clients.Kube.CoreV1().Pods(ns).GetLogs(pods.Items[0].Name, &corev1.PodLogOptions{SinceTime: &sinceTime})
	stream, err := req.Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("stream logs: %w", err)
	}
	defer func() { _ = stream.Close() }()
	var b strings.Builder
	if _, err := io.Copy(&b, io.LimitReader(stream, 1<<20)); err != nil {
		return "", fmt.Errorf("read logs: %w", err)
	}
	return b.String(), nil
}

// podLogs returns the tail of logs from the first pod matching selector in ns.
func (s *E2ESuite) podLogs(ctx context.Context, ns, selector string, tail int64) (string, error) {
	pods, err := s.clients.Kube.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return "", fmt.Errorf("list pods %s in %s: %w", selector, ns, err)
	}
	if len(pods.Items) == 0 {
		return "", fmt.Errorf("no pods match %s in %s", selector, ns)
	}
	tailLines := tail
	req := s.clients.Kube.CoreV1().Pods(ns).GetLogs(pods.Items[0].Name, &corev1.PodLogOptions{TailLines: &tailLines})
	stream, err := req.Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("stream logs: %w", err)
	}
	defer func() { _ = stream.Close() }()
	var b strings.Builder
	if _, err := io.Copy(&b, io.LimitReader(stream, 1<<20)); err != nil {
		return "", fmt.Errorf("read logs: %w", err)
	}
	return b.String(), nil
}

// rbacDefaultIncludesCreator reports whether the deployed api-server grants the
// gateway:creator role by default (so a plain developer may create gateways). It
// reads RBAC_DEFAULT_ROLES on the api-server deployment; unset means the Kind
// default, which includes gateway:creator.
func (s *E2ESuite) rbacDefaultIncludesCreator(t *testing.T) bool {
	dep, err := s.clients.Kube.AppsV1().Deployments(s.driver.PlatformNamespace()).Get(t.Context(), "hypershell-api-server", metav1.GetOptions{})
	if err != nil {
		t.Logf("WARN read api-server RBAC default (assuming creator-default): %v", err)
		return true
	}
	for _, c := range dep.Spec.Template.Spec.Containers {
		if c.Name != "api-server" {
			continue
		}
		for _, e := range c.Env {
			if e.Name == "RBAC_DEFAULT_ROLES" {
				return strings.Contains(e.Value, "gateway:creator")
			}
		}
	}
	return true // unset -> Kind default includes gateway:creator
}

// waitNamespaceGone polls until the namespace no longer exists.
func (s *E2ESuite) waitNamespaceGone(ctx context.Context, ns string, timeout time.Duration) error {
	return harness.Poll(ctx, 5*time.Second, timeout, func(ctx context.Context) (bool, error) {
		_, err := s.clients.Kube.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, nil
	})
}

// seedOrphanNamespace creates a synthetic managed namespace that the periodic GC
// reaper should delete: it carries the managed labels and a back-dated
// gc-eligible-since annotation so it is immediately eligible.
func (s *E2ESuite) seedOrphanNamespace(t *testing.T) string {
	name := "openshell-e2e-orphan-" + s.runID
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"hypershell.redhat.io/managed":  "true",
				"app.kubernetes.io/managed-by":  "hypershell-control-plane",
				"hypershell.redhat.io/instance": s.driver.PlatformNamespace(),
			},
			Annotations: map[string]string{
				"hypershell.redhat.io/gc-eligible-since": time.Now().Add(-3 * time.Minute).UTC().Format(time.RFC3339),
			},
		},
	}
	s.runner.Show("kubectl create namespace %s  # synthetic orphan for GC reaper", name)
	_, err := s.clients.Kube.CoreV1().Namespaces().Create(t.Context(), ns, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return name
	}
	s.Require().NoError(err, "seed orphan namespace")
	return name
}

// startOrphanReaper seeds the long-mode synthetic orphan as soon as the primary
// gateway gate passes and waits for its deletion in the background. P2.3 joins
// the buffered result, so the periodic sweep interval is off the critical path.
func (s *E2ESuite) startOrphanReaper(t *testing.T) {
	if s.mode != modeLong || s.orphan != nil {
		return
	}
	name := s.seedOrphanNamespace(t)
	done := make(chan error, 1)
	ctx, cancel := context.WithCancel(s.ctx)
	s.orphan = &orphanReaper{name: name, done: done, cancel: cancel}
	timeout := durationSecondsEnv("E2E_ORPHAN_GC_TIMEOUT", 90*time.Second)
	go func() {
		done <- s.waitNamespaceGone(ctx, name, timeout)
	}()
}

// assertGarbageCollectedEvent asserts a GarbageCollected Event was recorded for the
// reaped orphan namespace in the platform namespace.
func (s *E2ESuite) assertGarbageCollectedEvent(t *testing.T, orphan string) {
	ns := s.driver.PlatformNamespace()
	err := harness.Poll(t.Context(), 3*time.Second, 30*time.Second, func(ctx context.Context) (bool, error) {
		events, err := s.clients.Kube.CoreV1().Events(ns).List(ctx, metav1.ListOptions{
			FieldSelector: "involvedObject.name=" + orphan + ",reason=GarbageCollected",
		})
		if err != nil {
			return false, nil
		}
		return len(events.Items) > 0, nil
	})
	s.Assert().NoError(err, "GarbageCollected Event for orphan namespace %s", orphan)
}

// waitPodRunning polls until a pod whose name contains substr is Running in ns.
func (s *E2ESuite) waitPodRunning(ctx context.Context, ns, substr string, timeout time.Duration) error {
	return harness.Poll(ctx, 5*time.Second, timeout, func(ctx context.Context) (bool, error) {
		pods, err := s.clients.Kube.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			return false, nil
		}
		for _, p := range pods.Items {
			if strings.Contains(p.Name, substr) && p.Status.Phase == corev1.PodRunning {
				return true, nil
			}
		}
		return false, nil
	})
}

// pollActiveSandboxCount polls the gateway's active_sandbox_count until it reaches
// want. The count is advisory and may lag, so it is polled rather than read once.
func (s *E2ESuite) pollActiveSandboxCount(t *testing.T, gatewayID string, want int32) {
	err := harness.Poll(t.Context(), 5*time.Second, 90*time.Second, func(ctx context.Context) (bool, error) {
		gw, err := s.admin.Gateways().Get(ctx, gatewayID)
		if err != nil {
			return false, nil
		}
		return gw.ActiveSandboxCount == want, nil
	})
	s.Require().NoErrorf(err, "gateway %s active_sandbox_count never reached %d", gatewayID, want)
}
