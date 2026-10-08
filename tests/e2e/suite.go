package e2e

import (
	"context"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/openshift-online/hypershell/tests/e2e/apiclient"
	"github.com/openshift-online/hypershell/tests/e2e/driver"
	"github.com/openshift-online/hypershell/tests/e2e/harness"
)

// Run modes. perf is not a valid E2E_MODE value; the performance harness sets it
// in-process. long is the default so a plain `go test` runs every step.
const (
	modeShort = "short"
	modeLong  = "long"
	modePerf  = "perf"
)

// step tags select which modes a step runs in.
const (
	tagShort = "short" // runs in short, long, and perf
	tagLong  = "long"  // runs in long only
)

// E2ESuite is the single testify suite that drives the ordered phase steps P0-P3
// as named subtests. Shared state (tokens, discovered endpoints, seeded ids) is
// established in SetupSuite and reused across steps; TearDownSuite restores GC
// timing, deletes the suite's gateways, and prints the final summary on every exit
// path.
type E2ESuite struct {
	suite.Suite

	driver   driver.E2EInfraDriver
	clients  *harness.Clients
	runner   *harness.CommandRunner
	reporter *harness.Reporter

	mode        string
	concurrency int
	ctx         context.Context
	runID       string
	// startedAt bounds log scans (for example the control-plane Unauthenticated
	// check) to activity during this run, ignoring stale pre-run log noise.
	startedAt time.Time

	apiHost    string
	adminToken driver.Token
	admin      *apiclient.Client
	clusterID  string
	releaseID  string

	// clusterIDOverride, when set (by the matrix runner), pins the suite to a
	// specific cluster id instead of discovering the seeded one. nameSuffix is
	// appended to the run id so each per-cluster suite's gateway names are unique.
	clusterIDOverride string
	nameSuffix        string

	// primary is the P1 gateway the read-only steps reuse.
	primary driver.GatewayRef
	// primaryGatewayToken is the per-gateway admin token acquired in P1.3 for the
	// CLI steps (P1.4/P1.5).
	primaryGatewayToken driver.Token
	// primaryLocalName is the CLI's local handle for the primary gateway.
	primaryLocalName string
	// caFilePath caches the cluster CA written to disk for the CLI's SSL_CERT_FILE.
	caFilePath string
	// cliKubeconfigPath caches a temp kubeconfig pinned to E2E_KUBECONTEXT for the
	// CLI wrapper's kubectl.
	cliKubeconfigPath string
	// openshiftCLIImage caches the gateway-version-matched openshell CLI image used
	// by the OpenShift CLI path.
	openshiftCLIImage string

	mu              sync.Mutex
	createdGateways []string // gateway ids to clean up on teardown
}

func newE2ESuite(d driver.E2EInfraDriver, clients *harness.Clients) *E2ESuite {
	return &E2ESuite{driver: d, clients: clients}
}

// SetupSuite establishes the shared state every step depends on: the run id, the
// discovered API host, the admin token and API client, and the seeded cluster and
// release ids. In long mode it shortens the controller's namespace-GC timing for
// the P2.3 orphan-reaper assertion. A failure here fails the run; TearDownSuite
// still runs.
func (s *E2ESuite) SetupSuite() {
	s.ctx = context.Background()
	s.startedAt = time.Now()
	s.runner = harness.NewCommandRunner(s.T().Logf)
	s.reporter = harness.NewReporter()
	s.mode = envOrDefault("E2E_MODE", modeLong)
	s.concurrency = intEnv("E2E_CONCURRENCY", 4)
	s.runID = strconv.FormatInt(time.Now().Unix()%100000, 10)
	if s.nameSuffix != "" {
		s.runID = s.runID + "-" + s.nameSuffix
	}

	s.T().Logf("e2e mode=%s concurrency=%d driver=%s runID=%s", s.mode, s.concurrency, s.driver.Name(), s.runID)

	apiHost, err := s.driver.DiscoverAPIHost(s.ctx)
	s.Require().NoError(err, "discover API host")
	s.apiHost = apiHost

	tok, err := s.driver.AcquireOIDCToken(s.ctx, s.adminCreds())
	s.Require().NoError(err, "acquire admin OIDC token")
	s.adminToken = tok

	api, err := s.driver.APIClient(s.ctx, tok)
	s.Require().NoError(err, "build admin API client")
	s.admin = api

	clusterID, releaseID, err := s.discoverSeedIDs(s.ctx)
	s.Require().NoError(err, "discover seeded cluster/release ids")
	s.clusterID, s.releaseID = clusterID, releaseID
	s.T().Logf("seed ids: cluster=%s release=%s", clusterID, releaseID)

	if s.mode == modeLong {
		interval := durationSecondsEnv("E2E_GATEWAY_NAMESPACE_GC_INTERVAL", 30*time.Second)
		grace := durationSecondsEnv("E2E_GATEWAY_NAMESPACE_GC_GRACE_PERIOD", 30*time.Second)
		s.runner.Comment("shorten controller namespace-GC timing to interval=%s grace=%s for the P2.3 orphan-reaper assertion", interval, grace)
		s.Require().NoError(s.driver.ConfigureNamespaceGCTiming(s.ctx, interval, grace), "configure namespace GC timing")
	}
}

// TearDownSuite restores GC timing, deletes the gateways the suite created, calls
// the driver cleanup hook, and prints the final summary. It is nil-safe so it can
// run even when SetupSuite failed partway, and it never panics on a cleanup error.
func (s *E2ESuite) TearDownSuite() {
	ctx := context.Background()

	if s.driver != nil {
		if err := s.driver.RestoreNamespaceGCTiming(ctx); err != nil {
			s.T().Logf("WARN restore namespace GC timing: %v", err)
		}
	}

	if s.admin != nil {
		for _, id := range s.gatewayIDs() {
			if err := s.admin.Gateways().Delete(ctx, id); err != nil {
				s.T().Logf("WARN delete gateway %s: %v", id, err)
			}
		}
	}

	if s.driver != nil {
		if err := s.driver.DeSeedTestUsers(ctx); err != nil {
			s.T().Logf("WARN de-seed test users: %v", err)
		}
	}

	if s.reporter != nil {
		s.T().Log(s.reporter.Summary())
	}
}

// TestPhases drives the phase steps in canonical order. P0 and P1 are the
// sequential fail-fast gate (Require); on any gate failure the run stops rather
// than cascading into dependent steps. P2 and P3 follow (the parallel concurrency
// model is layered in a later wave; today they run sequentially, which is the
// defined E2E_CONCURRENCY=1 behavior).
func (s *E2ESuite) TestPhases() {
	// Phase P0 -- preflight gate.
	s.gate("P0.1", "authentication", tagShort, s.p0_1Authentication)
	s.gate("P0.2", "environment-readiness", tagShort, s.p0_2EnvironmentReadiness)

	// Phase P1 -- primary gateway up and reachable.
	s.gate("P1.1", "provisioning", tagShort, s.p1_1Provisioning)
	s.gate("P1.2", "infra-verification", tagShort, s.p1_2InfraVerification)
	s.gate("P1.3", "token-ca-trust", tagShort, s.p1_3TokenAndCATrust)
	s.gate("P1.4", "route-cli-registration", tagShort, s.p1_4RouteAndCLIRegistration)
	s.gate("P1.5", "connectivity", tagShort, s.p1_5Connectivity)

	// Phase P2 -- hard behaviors. (Parallel fan-out lands in a later wave.)
	s.step("P2.1", "sandbox-lifecycle", tagShort, s.p2_1SandboxLifecycle)
	s.step("P2.2", "rbac-enforcement", tagLong, s.p2_2RBACEnforcement)
	s.step("P2.3", "deletion-namespace-gc", tagShort, s.p2_3DeletionAndGC)
	s.step("P2.4", "managedcluster-lifecycle", tagLong, s.p2_4ManagedClusterLifecycle)
	s.step("P2.5", "release-promotion", tagLong, s.p2_5ReleasePromotion)

	// Phase P3 -- read-only verification.
	s.step("P3.1", "admin-inventory-validation", tagLong, s.p3_1AdminInventory)
}

// gate runs a step on the sequential fail-fast critical path: if it fails, the
// orchestrator stops so dependent steps do not run against a broken precondition.
func (s *E2ESuite) gate(id, slug, tag string, fn func(t *testing.T)) {
	s.step(id, slug, tag, fn)
	if s.T().Failed() {
		s.T().FailNow()
	}
}

// step runs one phase step as a named subtest, applies mode gating (a step not in
// the active mode is skipped, never silently passed), and records the pass/fail/skip
// outcome with the reporter.
func (s *E2ESuite) step(id, slug, tag string, fn func(t *testing.T)) {
	name := id + "-" + slug
	s.Run(name, func() {
		t := s.T()
		if !s.modeAllows(tag) {
			s.reporter.Record(id, slug, harness.OutcomeSkip, "not in mode "+s.mode)
			t.Skipf("step %s is %s-only; mode is %s", id, tag, s.mode)
			return
		}
		defer func() {
			switch {
			case t.Failed():
				s.reporter.Record(id, slug, harness.OutcomeFail, "")
			case t.Skipped():
				s.reporter.Record(id, slug, harness.OutcomeSkip, "")
			default:
				s.reporter.Record(id, slug, harness.OutcomePass, "")
			}
		}()
		// Refresh the admin token at the start of each step so a long run (whose
		// earlier steps can exceed the token TTL) does not hit 401s later.
		s.refreshAdminToken()
		fn(t)
	})
}

// refreshAdminToken re-acquires the admin token and rebuilds the admin API client.
// Best-effort: on failure it leaves the existing client in place and the step
// surfaces any resulting auth error.
func (s *E2ESuite) refreshAdminToken() {
	if s.driver == nil {
		return
	}
	tok, err := s.driver.AcquireOIDCToken(s.ctx, s.adminCreds())
	if err != nil {
		return
	}
	api, err := s.driver.APIClient(s.ctx, tok)
	if err != nil {
		return
	}
	s.adminToken = tok
	s.admin = api
}

// modeAllows reports whether a step with the given tag runs in the active mode.
func (s *E2ESuite) modeAllows(tag string) bool {
	if tag == tagShort {
		return true // short-tagged steps run in every mode
	}
	// long-tagged steps run only in long mode.
	return s.mode == modeLong
}

// --- shared helpers ---

func (s *E2ESuite) adminCreds() driver.Credentials {
	return driver.Credentials{
		Username: envOrDefault("E2E_OIDC_USERNAME", "admin"),
		Password: envOrDefault("E2E_OIDC_PASSWORD", "admin"),
	}
}

func (s *E2ESuite) devCreds() driver.Credentials {
	return driver.Credentials{
		Username: envOrDefault("E2E_DEV_USERNAME", "developer"),
		Password: envOrDefault("E2E_DEV_PASSWORD", "developer"),
	}
}

func (s *E2ESuite) platformAdminCreds() driver.Credentials {
	return driver.Credentials{
		Username: envOrDefault("E2E_PLATFORM_ADMIN_USERNAME", "platform-admin"),
		Password: envOrDefault("E2E_PLATFORM_ADMIN_PASSWORD", "platform-admin"),
	}
}

// trackGateway records a gateway id for teardown cleanup.
func (s *E2ESuite) trackGateway(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.createdGateways = append(s.createdGateways, id)
}

func (s *E2ESuite) gatewayIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.createdGateways))
	copy(out, s.createdGateways)
	return out
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func intEnv(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// durationSecondsEnv reads a duration env var where a bare integer means seconds
// (the Bash contract), falling back to def.
func durationSecondsEnv(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	if n, err := strconv.Atoi(v); err == nil {
		return time.Duration(n) * time.Second
	}
	return def
}

// seedDefaults returns the driver-specific default seed names.
func (s *E2ESuite) seedDefaults() (clusterName, releaseName string) {
	clusterName = "local-kind"
	if s.driver.Name() == "openshift" {
		clusterName = "local-openshift"
	}
	return clusterName, "dev-release"
}
