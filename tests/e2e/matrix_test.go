package e2e

import (
	"os"
	"slices"
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
//
// E2E_MATRIX_REQUIRED_CAPABILITY=<label> limits the matrix to the clusters that opt
// in by advertising <label>, and appends <label> to every suite's placement selector
// (E2E-CORE-30.05).
// Verifies: E2E-CORE-30.01, E2E-CORE-30.02, E2E-CORE-30.03, E2E-CORE-30.04, E2E-CORE-30.05
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
	//
	// E2E_MATRIX_REQUIRED_CAPABILITY, when set, narrows the matrix to the clusters
	// that opt in by advertising that label (E2E-CORE-30.05). Only their
	// capabilities are enumerated, and every suite also places by the label, so a
	// gateway can never land on a cluster that did not opt in (for example an
	// experimental spoke). Unset keeps the whole registered fleet.
	required := strings.TrimSpace(os.Getenv("E2E_MATRIX_REQUIRED_CAPABILITY"))
	clusters := make([]matrixCluster, 0, len(list.Items))
	for _, c := range list.Items {
		clusters = append(clusters, matrixCluster{ID: c.ID, OidcSubject: c.OidcSubject, Capabilities: c.Capabilities})
	}
	fleet := buildMatrixFleet(clusters, required)
	eligible := fleet.eligible
	registered := fleet.optedIn
	capabilities := fleet.capabilities()
	if required != "" {
		t.Logf("matrix limited to clusters advertising %s: %d of %d registered cluster(s) opted in", required, fleet.optedIn, fleet.registered)
	}
	if len(capabilities) == 0 {
		if required != "" {
			t.Fatalf("no distinct capabilities advertised by any registered managed cluster that advertises %s (opted in=%d, registered=%d of %d)", required, fleet.optedIn, fleet.registered, len(list.Items))
		}
		t.Fatalf("no distinct capabilities advertised by any registered managed cluster (registered=%d of %d)", fleet.registered, len(list.Items))
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
			s.matrixRequiredCapability = required
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

// matrixCluster is the slice of a ManagedCluster the matrix enumerates.
type matrixCluster struct {
	ID           string
	OidcSubject  string
	Capabilities []string
}

// matrixFleet is the set of clusters the matrix covers and the capabilities they
// advertise.
type matrixFleet struct {
	// eligible maps each capability to the ids of the covered clusters advertising it.
	eligible map[string]map[string]bool
	// registered counts clusters with a non-empty oidc_subject.
	registered int
	// optedIn counts the registered clusters the matrix covers: all of them when no
	// required capability is set, otherwise those that advertise it.
	optedIn int
}

// buildMatrixFleet enumerates the capabilities of the registered clusters the matrix
// covers (E2E-CORE-30). With a non-empty required capability it keeps only clusters
// that advertise that label and leaves the label itself out of the capability set:
// every covered cluster carries it and every suite already places by it, so a suite
// of its own would repeat another suite (E2E-CORE-30.05).
func buildMatrixFleet(clusters []matrixCluster, required string) matrixFleet {
	fleet := matrixFleet{eligible: map[string]map[string]bool{}}
	for _, c := range clusters {
		if c.OidcSubject == "" {
			continue
		}
		fleet.registered++
		if required != "" && !slices.Contains(c.Capabilities, required) {
			continue
		}
		fleet.optedIn++
		for _, capability := range c.Capabilities {
			if capability == required {
				continue
			}
			if fleet.eligible[capability] == nil {
				fleet.eligible[capability] = map[string]bool{}
			}
			fleet.eligible[capability][c.ID] = true
		}
	}
	return fleet
}

// capabilities returns the covered capabilities in deterministic order: at
// E2E_CONCURRENCY=1 the suites run sequentially in this order; higher values fan
// them out within that bound (E2E-CORE-30).
func (f matrixFleet) capabilities() []string {
	out := make([]string, 0, len(f.eligible))
	for capability := range f.eligible {
		out = append(out, capability)
	}
	sort.Strings(out)
	return out
}
