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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	sdktypes "github.com/openshift-online/hypershell/components/sdk-go/types"
	"github.com/openshift-online/hypershell/tests/e2e/driver"
	"github.com/openshift-online/hypershell/tests/e2e/harness"
)

// discoverSeedIDs resolves the seeded managed-cluster and gateway-release ids the
// suite provisions gateways against, selecting by E2E_SEED_CLUSTER_NAME /
// E2E_SEED_RELEASE_NAME (driver-specific defaults) and requiring a registered
// cluster (non-empty oidc_subject).
func (s *E2ESuite) discoverSeedIDs(ctx context.Context) (clusterID, releaseID string, err error) {
	defClusterName, defReleaseName := s.seedDefaults()
	clusterName := envOrDefault("E2E_SEED_CLUSTER_NAME", defClusterName)
	releaseName := envOrDefault("E2E_SEED_RELEASE_NAME", defReleaseName)

	clusters, err := s.admin.ManagedClusters().List(ctx, nil)
	if err != nil {
		return "", "", fmt.Errorf("list managed clusters: %w", err)
	}
	for _, c := range clusters.Items {
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
		return "", "", fmt.Errorf("no registered managed cluster found (wanted %q; re-seed with `make %s-seed`)", clusterName, s.driver.Name())
	}

	releases, err := s.admin.GatewayReleases().List(ctx, nil)
	if err != nil {
		return "", "", fmt.Errorf("list gateway releases: %w", err)
	}
	for _, r := range releases.Items {
		if r.Name == releaseName {
			releaseID = r.ID
			break
		}
		if releaseID == "" {
			releaseID = r.ID
		}
	}
	if releaseID == "" {
		return "", "", fmt.Errorf("no gateway release found (wanted %q; re-seed with `make %s-seed`)", releaseName, s.driver.Name())
	}
	return clusterID, releaseID, nil
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
	defer resp.Body.Close()
	s.Assert().Equal(http.StatusFound, resp.StatusCode, "BFF /auth/login must 302")
	s.Assert().Contains(resp.Header.Get("Location"), "code_challenge_method=S256", "PKCE challenge method in redirect")
}

func (s *E2ESuite) assertNoUnauthenticatedControllerLogs(t *testing.T) {
	logs, err := s.podLogs(t.Context(), s.driver.PlatformNamespace(), "app=hypershell-controller", 100)
	if err != nil {
		t.Logf("WARN read controller logs: %v", err)
		return
	}
	s.Assert().NotContains(logs, "Unauthenticated", "control plane gRPC watch must have no Unauthenticated errors")
}

// p0_2EnvironmentReadiness asserts the platform dependencies are healthy and the
// seed ids resolved (done in SetupSuite).
func (s *E2ESuite) p0_2EnvironmentReadiness(t *testing.T) {
	ctx := t.Context()
	s.assertDeploymentReady(t, "cert-manager", "cert-manager")
	s.assertDeploymentReady(t, "cert-manager", "cert-manager-webhook")
	s.assertDeploymentReady(t, "agent-sandbox-system", "agent-sandbox-controller")
	s.assertDeploymentReady(t, envOrDefault("E2E_KEYCLOAK_NAMESPACE", "keycloak"), "keycloak")

	served, err := s.clients.ServesAPIGroup("gateway.networking.k8s.io")
	s.Require().NoError(err, "query gateway.networking.k8s.io API group")
	s.Assert().True(served, "Gateway API (gateway.networking.k8s.io) must be served")

	nps, err := s.clients.Kube.NetworkingV1().NetworkPolicies(s.driver.PlatformNamespace()).List(ctx, metav1.ListOptions{})
	s.Require().NoError(err, "list network policies")
	s.Assert().GreaterOrEqual(len(nps.Items), 4, "platform namespace should have the baseline NetworkPolicies")

	s.Assert().NotEmpty(s.clusterID, "seeded cluster id")
	s.Assert().NotEmpty(s.releaseID, "seeded release id")
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
}

func (s *E2ESuite) p1_2InfraVerification(t *testing.T) {
	ctx := t.Context()
	ns := s.primary.Namespace
	s.Require().NotEmpty(ns, "primary gateway namespace")

	dep, err := s.clients.Kube.AppsV1().Deployments(ns).Get(ctx, "openshell-gateway", metav1.GetOptions{})
	s.Require().NoError(err, "get gateway deployment")
	s.Assert().GreaterOrEqual(int(dep.Status.ReadyReplicas), 1, "gateway deployment ready replicas")

	svc, err := s.clients.Kube.CoreV1().Services(ns).Get(ctx, "openshell-gateway", metav1.GetOptions{})
	s.Require().NoError(err, "get gateway service")
	s.Assert().NotEmpty(svc.Spec.ClusterIP, "gateway service ClusterIP")

	// TLS is provisioned by cert-manager Certificates (openshell-gateway-server),
	// not a certgen Job; the populated server-TLS secret is the Certificate's
	// output and the evidence the issuance succeeded. (Divergence from the spec's
	// "certgen job" wording, which predates the cert-manager implementation.)
	tlsSecret, err := s.clients.Kube.CoreV1().Secrets(ns).Get(ctx, "openshell-server-tls", metav1.GetOptions{})
	s.Require().NoError(err, "gateway TLS secret openshell-server-tls")
	s.Assert().NotEmpty(tlsSecret.Data["tls.crt"], "server TLS cert populated")
	s.Assert().NotEmpty(tlsSecret.Data["tls.key"], "server TLS key populated")
}

func (s *E2ESuite) p1_3TokenAndCATrust(t *testing.T) {
	// The per-gateway Keycloak client is "<gw-name>-<gw-id>"; block until the
	// admin role lands in a token for it (roles reconcile asynchronously).
	clientID := fmt.Sprintf("%s-%s", s.primary.Name, s.primary.ID)
	s.runner.Comment("acquire per-gateway token for client %s with role openshell-admin", clientID)
	tok, err := s.driver.AcquireGatewayTokenWithRole(t.Context(), s.adminCreds(), clientID, "openshell-admin")
	s.Require().NoError(err, "acquire gateway token with openshell-admin role")
	s.Assert().NotEmpty(tok.AccessToken, "gateway token")
	// CA trust is already installed process-wide (no insecure bypass); nothing else
	// to assert here until the CLI steps consume SSL_CERT_FILE in a later wave.
}

func (s *E2ESuite) p1_4RouteAndCLIRegistration(t *testing.T) {
	// Route discovery is infra-level and testable now; the openshell CLI
	// registration lands with the CLI integration wave. The GRPCRoute is created
	// shortly after the gateway reaches Running, so wait for it before discovering.
	s.Require().NoError(s.driver.WaitForGatewayRoute(t.Context(), s.primary), "wait for gateway route")
	endpoint, err := s.driver.DiscoverGatewayEndpoint(t.Context(), s.primary)
	s.Require().NoError(err, "discover gateway endpoint")
	t.Logf("gateway endpoint: %s", endpoint)
	t.Skip("openshell CLI registration pending CLI-integration wave")
}

func (s *E2ESuite) p1_5Connectivity(t *testing.T) {
	t.Skip("openshell CLI connectivity pending CLI-integration wave")
}

// --- Phase P2 / P3 (pending implementation waves) ---

func (s *E2ESuite) p2_1SandboxLifecycle(t *testing.T)        { t.Skip("pending CLI-integration wave") }
func (s *E2ESuite) p2_2RBACEnforcement(t *testing.T)         { t.Skip("pending P2 wave") }
func (s *E2ESuite) p2_3DeletionAndGC(t *testing.T)           { t.Skip("pending P2 wave") }
func (s *E2ESuite) p2_4ManagedClusterLifecycle(t *testing.T) { t.Skip("pending P2 wave") }
func (s *E2ESuite) p2_5ReleasePromotion(t *testing.T)        { t.Skip("pending P2 wave") }
func (s *E2ESuite) p3_1AdminInventory(t *testing.T)          { t.Skip("pending P3 wave") }

// --- low-level helpers ---

// createGateway provisions a gateway via the create endpoint, sending only the
// create-request fields (name, cluster_id, release_id). It uses the raw helper
// rather than the SDK's Create, which also serializes server-managed fields (for
// example the readOnly namespace) that the strict create endpoint rejects.
func (s *E2ESuite) createGateway(t *testing.T, name string) *sdktypes.Gateway {
	// route and oidc are JSON-encoded string fields (matching the seed and the API
	// schema). route.enabled=true is what makes the control plane create the
	// gateway's GRPCRoute/HTTPRoute; without it the gateway reaches Running but has
	// no external route. oidc (issuer/audience) is driver-specific and threaded in
	// with the CLI-connect steps in a later wave.
	body := map[string]string{
		"name":       name,
		"cluster_id": s.clusterID,
		"release_id": s.releaseID,
		"route":      `{"enabled":true}`,
	}
	s.runner.Show("POST %s/api/hypershell/v1/gateways  # name=%s cluster=%s release=%s", s.apiHost, name, s.clusterID, s.releaseID)
	status, resp, err := s.admin.RawJSON(t.Context(), http.MethodPost, "/gateways", body)
	s.Require().NoError(err, "create gateway request")
	s.Require().Equalf(http.StatusCreated, status, "create gateway status (body: %s)", string(resp))

	var created sdktypes.Gateway
	s.Require().NoError(json.Unmarshal(resp, &created), "unmarshal created gateway")
	s.Require().NotEmpty(created.ID, "created gateway id")
	return &created
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
		phase := "<unknown>"
		if last != nil {
			phase = last.Phase
		}
		s.Require().NoError(err, "gateway %s never reached Running (last phase %s)", id, phase)
	}
	return last
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
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, body, nil
}

func (s *E2ESuite) assertDeploymentReady(t *testing.T, ns, name string) {
	dep, err := s.clients.Kube.AppsV1().Deployments(ns).Get(t.Context(), name, metav1.GetOptions{})
	if !s.Assert().NoError(err, "get deployment %s/%s", ns, name) {
		return
	}
	s.Assert().GreaterOrEqual(int(dep.Status.ReadyReplicas), 1, "deployment %s/%s ready", ns, name)
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
	defer stream.Close()
	var b strings.Builder
	if _, err := io.Copy(&b, io.LimitReader(stream, 1<<20)); err != nil {
		return "", fmt.Errorf("read logs: %w", err)
	}
	return b.String(), nil
}
