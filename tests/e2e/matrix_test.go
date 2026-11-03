package e2e

import (
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/openshift-online/hypershell/tests/e2e/driver"
	"github.com/openshift-online/hypershell/tests/e2e/harness"
)

// TestMatrix is the managed-cluster matrix runner. It validates placement PER
// DISTINCT CAPABILITY advertised across the registered fleet, not per cluster, so its
// cost scales with the bounded capability set rather than the cluster count: "does
// placement by this capability work" is a property of the software and the fleet's
// capability set, so running the full suite once per cluster is redundant at fleet
// scale and does not scale to 100+ clusters (E2E-CORE-30).
//
// It reads GET /managed_clusters, enumerates the union of distinct capability labels
// the registered clusters (non-empty oidc_subject) advertise, and runs the E2ESuite
// in short mode once per distinct capability. Each suite places its positive-workflow
// and provisioning gateways by that one capability selector
// (placement.selector = [<capability>]) and asserts the gateway lands on some
// eligible cluster. Across the run the matrix asserts full capability coverage and
// reports the observational landed-cluster union (never a failure). The suite count
// equals the distinct-capability count and is independent of the cluster count.
//
// Short mode is user-facing and kube-free, so the matrix needs no Kubernetes context
// for any cluster and applies no last_seen_at health gate; it enumerates only
// registered clusters as a structural placement-eligibility precondition, not a
// health judgment (E2E-CORE-30.04, E2E-CORE-39).
// Verifies: E2E-CORE-30.01, E2E-CORE-30.02, E2E-CORE-30.03, E2E-CORE-30.04
func TestMatrix(t *testing.T) {
	// The matrix is the user-facing "does placement by this capability work" gate; it
	// always runs short mode (the kube-free depth), uniformly across hub and spokes.
	t.Setenv("E2E_MODE", modeShort)

	clients, err := harness.NewClients()
	if err != nil {
		t.Fatalf("build Kubernetes clients: %v", err)
	}
	d, err := driver.Resolve(t.Context(), clients, os.Getenv("E2E_INFRA_DRIVER"))
	if err != nil {
		t.Fatalf("resolve infra driver: %v", err)
	}

	ctx := t.Context()
	adminTok, err := d.AcquireOIDCToken(ctx, driver.Credentials{
		Username: envOrDefault("E2E_OIDC_USERNAME", "admin"),
		Password: envOrDefault("E2E_OIDC_PASSWORD", "admin"),
	})
	if err != nil {
		t.Fatalf("acquire admin token: %v", err)
	}
	api, err := d.APIClient(ctx, adminTok)
	if err != nil {
		t.Fatalf("build API client: %v", err)
	}
	list, err := api.ManagedClusters().List(ctx, nil)
	if err != nil {
		t.Fatalf("list managed clusters: %v", err)
	}

	// Enumerate the distinct capabilities advertised by REGISTERED clusters only
	// (non-empty oidc_subject). An unregistered cluster has no control plane to
	// reconcile a gateway and is never a placement target, so enumerating its
	// capabilities would manufacture a guaranteed-unplaceable capability rather than
	// exercise a real placement. This is a structural placement-eligibility
	// precondition, NOT a health judgment: no last_seen_at gate is applied.
	eligible := map[string]map[string]bool{} // capability -> set of cluster ids advertising it
	registered := 0
	for _, c := range list.Items {
		if c.OidcSubject == "" {
			continue
		}
		registered++
		for _, capability := range c.Capabilities {
			if eligible[capability] == nil {
				eligible[capability] = map[string]bool{}
			}
			eligible[capability][c.ID] = true
		}
	}
	capabilities := make([]string, 0, len(eligible))
	for capability := range eligible {
		capabilities = append(capabilities, capability)
	}
	// Deterministic capability order: at E2E_CONCURRENCY=1 the suites run sequentially
	// in this order; higher values fan them out within that bound (E2E-CORE-30).
	sort.Strings(capabilities)
	if len(capabilities) == 0 {
		t.Fatalf("no distinct capabilities advertised by any registered managed cluster (registered=%d of %d)", registered, len(list.Items))
	}

	concurrency := intEnv("E2E_CONCURRENCY", 4)
	if concurrency < 1 {
		concurrency = 1
	}

	var mu sync.Mutex
	outcomes := map[string]string{}
	// exercised records the capabilities the matrix actually ran a suite for, so the
	// run can assert full capability coverage: exercised must equal the distinct
	// capability set (E2E-CORE-30.02).
	exercised := map[string]bool{}
	// landed records the clusters positive creates resolved to across all suites, for
	// the observational landed-cluster union (E2E-CORE-30.02).
	landed := &landedTracker{}

	sem := make(chan struct{}, concurrency)
	for _, capability := range capabilities {
		capability := capability
		t.Run(capability, func(t *testing.T) {
			if concurrency > 1 {
				t.Parallel()
				select {
				case sem <- struct{}{}:
					defer func() { <-sem }()
				case <-t.Context().Done():
					return
				}
			}

			s := newE2ESuite(d, clients)
			// Place positive creates by this one capability selector and assert each
			// gateway lands on some cluster that advertises it (E2E-CORE-30). The suite
			// runs short mode, so it never opens a per-cluster kube context.
			s.matrixCapability = capability
			s.matrixEligibleClusters = eligible[capability]
			s.landed = landed
			s.nameSuffix = sanitizeCapability(capability)

			mu.Lock()
			exercised[capability] = true
			mu.Unlock()

			suite.Run(t, s)

			mu.Lock()
			if t.Failed() {
				outcomes[capability] = "FAIL"
			} else {
				outcomes[capability] = "PASS"
			}
			mu.Unlock()
		})
	}

	t.Cleanup(func() {
		mu.Lock()
		defer mu.Unlock()
		var b strings.Builder
		b.WriteString("\n==================== E2E MATRIX (per capability) ====================\n")
		for _, capability := range capabilities {
			res := outcomes[capability]
			if res == "" {
				res = "UNKNOWN"
			}
			b.WriteString("  " + capability + ": " + res + "\n")
		}
		// The landed-cluster union is observational, never a coverage assertion: a
		// single cluster MAY satisfy several capabilities and some clusters MAY never
		// be landed on, which is expected (E2E-CORE-30.02).
		b.WriteString("  landed-cluster union: " + strings.Join(landed.list(), ", ") + "\n")
		b.WriteString("  cost: " + strconv.Itoa(len(capabilities)) + " suite(s) for " + strconv.Itoa(registered) + " registered cluster(s)\n")
		b.WriteString("=====================================================================\n")
		t.Log(b.String())

		// Full capability coverage: every distinct capability present across the
		// registered clusters is exercised by exactly one suite. This guards against a
		// capability being silently dropped (for example a panic before suite.Run) so a
		// green matrix cannot hide an unexercised capability (E2E-CORE-30.02).
		var missing []string
		for _, capability := range capabilities {
			if !exercised[capability] {
				missing = append(missing, capability)
			}
		}
		if len(missing) > 0 {
			t.Errorf("matrix did not exercise every distinct capability; missing: %s", strings.Join(missing, ", "))
		}
	})
}

// sanitizeCapability makes a capability label safe as part of a gateway name
// (provider/aws -> provider-aws).
func sanitizeCapability(capability string) string {
	r := strings.NewReplacer("/", "-", ".", "-", ":", "-", " ", "-", "_", "-")
	return strings.ToLower(r.Replace(capability))
}
