package e2e

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"golang.org/x/sync/errgroup"

	sdktypes "github.com/openshift-online/hypershell/components/sdk-go/types"
	"github.com/openshift-online/hypershell/tests/e2e/apiclient"
	"github.com/openshift-online/hypershell/tests/e2e/driver"
	"github.com/openshift-online/hypershell/tests/e2e/harness"
)

// Run modes. perf is retained only as an explicitly invalid value from the former
// shell harness; performance now uses a standalone Go benchmark. long is the
// default so a plain `go test` runs every step.
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
// timing and deletes the suite's gateways. Go's native test output is the result
// report.
type E2ESuite struct {
	suite.Suite

	driver  driver.E2EInfraDriver
	clients *harness.Clients
	runner  *harness.CommandRunner

	mode        string
	concurrency int
	ctx         context.Context
	runID       string
	// startedAt bounds log scans (for example the control-plane Unauthenticated
	// check) to activity during this run, ignoring stale pre-run log noise.
	startedAt time.Time

	apiHost   string
	admin     *apiclient.Client
	clusterID string

	// clusterIDOverride, when set (by the matrix runner), pins the suite to a
	// specific cluster id instead of discovering the seeded one. nameSuffix is
	// appended to the run id so each per-cluster suite's gateway names are unique.
	clusterIDOverride string
	nameSuffix        string
	kubeContext       string
	skipKubeChecks    bool

	// primary is the P1 gateway the read-only steps reuse.
	primary driver.GatewayRef
	// primaryGatewayToken is the per-gateway admin token acquired in P1.3 for the
	// CLI steps (P1.4/P1.5).
	primaryGatewayToken driver.Token
	// primaryLocalName is the CLI's local handle for the primary gateway.
	primaryLocalName string
	gateways         *gatewayTracker
	cliCache         *cliCache
	orphan           *orphanReaper
	gcTimingLease    *namespaceGCTimingLease
	controllerReady  *completionBarrier
}

type gatewayTracker struct {
	mu  sync.Mutex
	ids []string
}

type cliCache struct {
	mu                sync.Mutex
	caFilePath        string
	cliKubeconfigPath string
	openshiftCLIImage string
}

type orphanReaper struct {
	name   string
	done   <-chan error
	cancel context.CancelFunc
}

func newE2ESuite(d driver.E2EInfraDriver, clients *harness.Clients) *E2ESuite {
	return &E2ESuite{
		driver:   d,
		clients:  clients,
		gateways: &gatewayTracker{},
		cliCache: &cliCache{},
	}
}

// SetupSuite establishes the shared state every step depends on: the run id, the
// discovered API host, the admin token and API client, and the seeded cluster id.
// In long mode it shortens the controller's namespace-GC timing for
// the P2.3 orphan-reaper assertion. A failure here fails the run; TearDownSuite
// still runs.
func (s *E2ESuite) SetupSuite() {
	s.ctx = s.T().Context()
	s.startedAt = time.Now()
	s.runner = harness.NewCommandRunner(s.T().Logf)
	s.mode = envOrDefault("E2E_MODE", modeLong)
	s.Require().Truef(validE2EMode(s.mode), "invalid E2E_MODE %q (valid values: %s, %s)", s.mode, modeShort, modeLong)
	s.concurrency = intEnv("E2E_CONCURRENCY", 4)
	if s.concurrency < 1 {
		s.concurrency = 1
	}
	s.runID = strconv.FormatInt(time.Now().Unix()%100000, 10)
	if s.nameSuffix != "" {
		s.runID = s.runID + "-" + s.nameSuffix
	}

	s.T().Logf("e2e mode=%s concurrency=%d driver=%s runID=%s", s.mode, s.concurrency, s.driver.Name(), s.runID)

	var apiHost string
	var tok driver.Token
	g, ctx := errgroup.WithContext(s.ctx)
	g.SetLimit(s.concurrency)
	g.Go(func() error {
		var err error
		apiHost, err = s.driver.DiscoverAPIHost(ctx)
		if err != nil {
			return fmt.Errorf("discover API host: %w", err)
		}
		return nil
	})
	if s.mode == modeLong {
		g.Go(func() error {
			for _, credentials := range []driver.Credentials{s.devCreds(), s.platformAdminCreds()} {
				if err := s.driver.CreateTestUser(ctx, credentials.Username, credentials.Password); err != nil {
					return fmt.Errorf("ensure long-mode test user %s: %w", credentials.Username, err)
				}
			}
			return nil
		})
	}
	g.Go(func() error {
		var err error
		tok, err = s.driver.AcquireOIDCToken(ctx, s.adminCreds())
		if err != nil {
			return fmt.Errorf("acquire admin OIDC token: %w", err)
		}
		return nil
	})
	if s.mode == modeLong && !s.skipKubeChecks {
		interval := durationSecondsEnv("E2E_GATEWAY_NAMESPACE_GC_INTERVAL", 30*time.Second)
		grace := durationSecondsEnv("E2E_GATEWAY_NAMESPACE_GC_GRACE_PERIOD", 30*time.Second)
		s.runner.Comment("shorten controller namespace-GC timing to interval=%s grace=%s for the P2.3 orphan-reaper assertion", interval, grace)
		g.Go(func() error {
			lease, err := acquireNamespaceGCTiming(ctx, s.driver, interval, grace)
			if err != nil {
				return fmt.Errorf("configure namespace GC timing: %w", err)
			}
			s.gcTimingLease = lease
			return nil
		})
	}
	s.Require().NoError(g.Wait(), "parallel suite setup")
	s.apiHost = apiHost

	api, err := s.driver.APIClient(s.ctx, tok)
	s.Require().NoError(err, "build admin API client")
	s.admin = api

	clusterID, err := s.discoverSeedIDs(s.ctx)
	s.Require().NoError(err, "discover seeded cluster id")
	s.clusterID = clusterID
	s.T().Logf("seed ids: cluster=%s", clusterID)

}

// TearDownSuite restores GC timing, deletes the gateways the suite created, calls
// the driver cleanup hook. It is nil-safe so it can run even when SetupSuite
// failed partway, and it never panics on a cleanup error.
func (s *E2ESuite) TearDownSuite() {
	ctx := context.Background()
	if s.orphan != nil && s.orphan.cancel != nil {
		s.orphan.cancel()
	}

	cleanupAPI := s.admin
	if s.driver != nil {
		if tok, err := s.driver.AcquireOIDCToken(ctx, s.adminCreds()); err == nil {
			if api, err := s.driver.APIClient(ctx, tok); err == nil {
				cleanupAPI = api
			}
		}
	}
	if cleanupAPI != nil && os.Getenv("E2E_SKIP_CLEANUP") != "1" {
		g, ctx := errgroup.WithContext(ctx)
		g.SetLimit(max(1, s.concurrency))
		for _, id := range s.gatewayIDs() {
			id := id
			g.Go(func() error {
				return harness.Poll(ctx, 2*time.Second, 2*time.Minute, func(ctx context.Context) (bool, error) {
					err := cleanupAPI.Gateways().Delete(ctx, id)
					if err == nil {
						return true, nil
					}
					var apiErr *sdktypes.APIError
					if errors.As(err, &apiErr) {
						if apiErr.StatusCode == http.StatusNotFound {
							return true, nil
						}
						if apiErr.StatusCode >= http.StatusInternalServerError {
							return false, nil
						}
					}
					return false, fmt.Errorf("delete gateway %s: %w", id, err)
				})
			})
		}
		if err := g.Wait(); err != nil {
			s.T().Errorf("gateway cleanup: %v", err)
		}
	}

	if s.gcTimingLease != nil && !(s.T().Failed() && s.driver.Name() == "openshift") {
		if err := s.gcTimingLease.Release(ctx); err != nil {
			s.T().Errorf("restore namespace GC timing: %v", err)
		}
	}

	if s.driver != nil {
		if err := s.driver.DeSeedTestUsers(ctx); err != nil {
			s.T().Errorf("de-seed test users: %v", err)
		}
	}

}

// TestPhases drives the phase steps in canonical order. P0 and P1 are the
// sequential fail-fast gate (Require); on any gate failure the run stops rather
// than cascading into dependent steps. P2 and P3 use bounded parallel subtests;
// E2E_CONCURRENCY=1 preserves strict canonical order for debugging.
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

	// Phase P2 -- hard behaviors. At concurrency > 1, start the known longest
	// steps first. At concurrency 1, retain canonical phase order.
	s.Require().NoError(s.refreshAdminToken(), "refresh admin token before P2")
	s.T().Run("Phase-P2", func(t *testing.T) {
		runParallelSubtests(t, s.concurrency, s.p2ParallelSteps())
	})

	// Phase P3 -- read-only verification. The bounded runner makes future P3
	// additions parallel without changing orchestration.
	s.Require().NoError(s.refreshAdminToken(), "refresh admin token before P3")
	s.T().Run("Phase-P3", func(t *testing.T) {
		runParallelSubtests(t, s.concurrency, []parallelSubtest{
			s.parallelStep("P3.1", "admin-inventory-validation", tagLong, (*E2ESuite).p3_1AdminInventory),
		})
	})
}

// p2ParallelSteps returns the hard-behavior wave in canonical order for a
// sequential debug run and dependency-aware order for the normal bounded
// fan-out. P2.4 begins in parallel, but its controller disconnect waits until
// the Keycloak/controller-sensitive P2.2, P2.5, and P2.6 steps have completed.
func (s *E2ESuite) p2ParallelSteps() []parallelSubtest {
	canonical := []parallelSubtest{
		s.parallelStep("P2.1", "sandbox-lifecycle", tagShort, (*E2ESuite).p2_1SandboxLifecycle),
		s.parallelStep("P2.2", "rbac-enforcement", tagLong, (*E2ESuite).p2_2RBACEnforcement),
		s.parallelStep("P2.3", "deletion-namespace-gc", tagShort, (*E2ESuite).p2_3DeletionAndGC),
		s.parallelStep("P2.4", "managedcluster-lifecycle", tagLong, (*E2ESuite).p2_4ManagedClusterLifecycle),
		s.parallelStep("P2.5", "gateway-access-management", tagLong, (*E2ESuite).p2_5GatewayAccessManagement),
		s.parallelStep("P2.6", "gateway-service-accounts", tagLong, (*E2ESuite).p2_6GatewayServiceAccounts),
	}
	if s.concurrency == 1 {
		return canonical
	}

	barrier := newCompletionBarrier(3)
	s.controllerReady = barrier
	markControllerSensitive := func(step parallelSubtest) parallelSubtest {
		run := step.run
		step.run = func(t *testing.T) {
			defer barrier.Done()
			run(t)
		}
		return step
	}

	// Queue the controller-sensitive work before P2.4 so its barrier cannot
	// consume the last scheduler slot while a required peer is still queued.
	return []parallelSubtest{
		canonical[0],
		canonical[2],
		markControllerSensitive(canonical[4]),
		markControllerSensitive(canonical[5]),
		markControllerSensitive(canonical[1]),
		canonical[3],
	}
}

// gate runs a step on the sequential fail-fast critical path: if it fails, the
// orchestrator stops so dependent steps do not run against a broken precondition.
func (s *E2ESuite) gate(id, slug, tag string, fn func(t *testing.T)) {
	s.Require().NoError(s.refreshAdminToken(), "refresh admin token before %s", id)
	s.step(id, slug, tag, fn)
	if s.T().Failed() {
		s.T().FailNow()
	}
}

// step runs one phase step as a named subtest and applies mode gating. A step not
// in the active mode is skipped rather than silently passed; Go's test output is
// the canonical per-step result report.
func (s *E2ESuite) step(id, slug, tag string, fn func(t *testing.T)) {
	name := id + "-" + slug
	s.Run(name, func() {
		t := s.T()
		if !s.modeAllows(tag) {
			t.Skipf("step %s is %s-only; mode is %s", id, tag, s.mode)
			return
		}
		fn(t)
	})
}

func (s *E2ESuite) parallelStep(id, slug, tag string, fn func(*E2ESuite, *testing.T)) parallelSubtest {
	return parallelSubtest{
		name: id + "-" + slug,
		run: func(t *testing.T) {
			if !s.modeAllows(tag) {
				t.Skipf("step %s is %s-only; mode is %s", id, tag, s.mode)
				return
			}
			stepSuite := s.cloneForStep(t)
			fn(stepSuite, t)
		},
	}
}

// cloneForStep gives a parallel subtest its own testify context, command logger,
// token, and API client. Cleanup, CLI caches, orphan state, and timing remain
// shared through explicitly synchronized pointers.
func (s *E2ESuite) cloneForStep(t *testing.T) *E2ESuite {
	t.Helper()
	tok, err := s.driver.AcquireOIDCToken(t.Context(), s.adminCreds())
	if err != nil {
		t.Fatalf("acquire step admin OIDC token: %v", err)
	}
	api, err := s.driver.APIClient(t.Context(), tok)
	if err != nil {
		t.Fatalf("build step admin API client: %v", err)
	}
	clone := &E2ESuite{
		driver:              s.driver,
		clients:             s.clients,
		runner:              harness.NewCommandRunner(t.Logf),
		mode:                s.mode,
		concurrency:         s.concurrency,
		ctx:                 s.ctx,
		runID:               s.runID,
		startedAt:           s.startedAt,
		apiHost:             s.apiHost,
		admin:               api,
		clusterID:           s.clusterID,
		clusterIDOverride:   s.clusterIDOverride,
		nameSuffix:          s.nameSuffix,
		primary:             s.primary,
		primaryGatewayToken: s.primaryGatewayToken,
		primaryLocalName:    s.primaryLocalName,
		gateways:            s.gateways,
		cliCache:            s.cliCache,
		orphan:              s.orphan,
		controllerReady:     s.controllerReady,
	}
	clone.SetT(t)
	return clone
}

// refreshAdminToken re-acquires the admin token and rebuilds the admin API client.
// Best-effort: on failure it leaves the existing client in place and the step
// surfaces any resulting auth error.
func (s *E2ESuite) refreshAdminToken() error {
	if s.driver == nil {
		return fmt.Errorf("infra driver is nil")
	}
	tok, err := s.driver.AcquireOIDCToken(s.ctx, s.adminCreds())
	if err != nil {
		return fmt.Errorf("acquire admin token: %w", err)
	}
	api, err := s.driver.APIClient(s.ctx, tok)
	if err != nil {
		return fmt.Errorf("build admin API client: %w", err)
	}
	s.admin = api
	return nil
}

// modeAllows reports whether a step with the given tag runs in the active mode.
func (s *E2ESuite) modeAllows(tag string) bool {
	if tag == tagShort {
		return true // short-tagged steps run in every mode
	}
	// long-tagged steps run only in long mode.
	return s.mode == modeLong
}

func validE2EMode(mode string) bool {
	return mode == modeShort || mode == modeLong
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
	s.gateways.mu.Lock()
	defer s.gateways.mu.Unlock()
	s.gateways.ids = append(s.gateways.ids, id)
}

func (s *E2ESuite) gatewayIDs() []string {
	s.gateways.mu.Lock()
	defer s.gateways.mu.Unlock()
	out := make([]string, len(s.gateways.ids))
	copy(out, s.gateways.ids)
	return out
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// gatewayTLSInsecure honors only an explicit diagnostic override. Both drivers
// install a trusted CA path, including Kind's local self-signed CA.
func gatewayTLSInsecure(_ string) bool {
	return os.Getenv("OPENSHELL_GATEWAY_INSECURE") == "true"
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

// seedDefaults returns the driver-specific default seed cluster name.
func (s *E2ESuite) seedDefaults() (clusterName string) {
	clusterName = "local-kind"
	if s.driver.Name() == "openshift" {
		clusterName = "local-openshift"
	}
	return clusterName
}
