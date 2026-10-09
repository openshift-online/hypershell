package e2e

import (
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/openshift-online/hypershell/tests/e2e/driver"
	"github.com/openshift-online/hypershell/tests/e2e/harness"
)

// TestMatrix is the managed-cluster matrix runner. It enumerates registered
// ManagedClusters and runs the single-cluster E2ESuite once per cluster, bounded
// by E2E_CONCURRENCY, then reports a per-cluster pass/fail/skip matrix and a single
// gate verdict (go test's exit status). It adds no assertions beyond the suite.
//
// It defaults to E2E_MODE=short (the promotion/smoke subset) when E2E_MODE is
// unset, and fails closed: an unregistered, stale, or unreachable cluster is a
// failure unless E2E_MANAGED_SKIP_UNHEALTHY=1 downgrades it to a skip.
func TestMatrix(t *testing.T) {
	// The matrix is a promotion/smoke gate: default to short unless the caller
	// pinned a mode.
	if os.Getenv("E2E_MODE") == "" {
		t.Setenv("E2E_MODE", modeShort)
	}

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

	allow := parseAllowlist(os.Getenv("E2E_MANAGED_CLUSTERS"))
	healthyWindow := durationEnvOr("E2E_MANAGED_HEALTHY_WINDOW", 5*time.Minute)
	skipUnhealthy := os.Getenv("E2E_MANAGED_SKIP_UNHEALTHY") == "1"
	concurrency := intEnv("E2E_CONCURRENCY", 4)
	if concurrency < 1 {
		concurrency = 1
	}

	type target struct {
		name, id string
		healthy  bool
		reason   string
	}
	var targets []target
	for _, c := range list.Items {
		if len(allow) > 0 && !allow[c.Name] {
			continue
		}
		if c.OidcSubject == "" {
			targets = append(targets, target{name: c.Name, id: c.ID, reason: "unregistered (empty oidc_subject)"})
			continue
		}
		healthy, reason := clusterHealthy(c.LastSeenAt, healthyWindow)
		targets = append(targets, target{name: c.Name, id: c.ID, healthy: healthy, reason: reason})
	}
	if len(targets) == 0 {
		t.Fatalf("no ManagedClusters selected (registered=%d, allowlist=%v)", len(list.Items), allow)
	}

	var mu sync.Mutex
	outcomes := map[string]string{}

	sem := make(chan struct{}, concurrency)
	for _, tg := range targets {
		tg := tg
		t.Run(tg.name, func(t *testing.T) {
			if concurrency > 1 {
				t.Parallel()
				select {
				case sem <- struct{}{}:
					defer func() { <-sem }()
				case <-t.Context().Done():
					return
				}
			}

			if !tg.healthy {
				if skipUnhealthy {
					mu.Lock()
					outcomes[tg.name] = "SKIP (" + tg.reason + ")"
					mu.Unlock()
					t.Skipf("cluster %s unhealthy: %s (E2E_MANAGED_SKIP_UNHEALTHY=1)", tg.name, tg.reason)
					return
				}
				mu.Lock()
				outcomes[tg.name] = "FAIL (" + tg.reason + ")"
				mu.Unlock()
				t.Fatalf("cluster %s unhealthy: %s (fail-closed; set E2E_MANAGED_SKIP_UNHEALTHY=1 to downgrade)", tg.name, tg.reason)
				return
			}

			clusterClients := clients
			clusterDriver := d
			contextName := os.Getenv(managedKubeContextEnvKey(tg.name))
			if contextName != "" {
				var contextErr error
				clusterClients, contextErr = harness.NewClientsForContext(contextName)
				if contextErr != nil {
					t.Fatalf("build Kubernetes clients for context %s: %v", contextName, contextErr)
				}
				targetDriver, resolveErr := driver.Resolve(t.Context(), clusterClients, d.Name())
				contextErr = resolveErr
				if contextErr != nil {
					t.Fatalf("resolve %s driver for context %s: %v", d.Name(), contextName, contextErr)
				}
				clusterDriver = matrixInfraDriver{hub: d, target: targetDriver}
			}

			s := newE2ESuite(clusterDriver, clusterClients)
			s.clusterIDOverride = tg.id
			s.nameSuffix = sanitizeName(tg.name)
			s.kubeContext = contextName
			s.skipKubeChecks = contextName == ""
			suite.Run(t, s)

			mu.Lock()
			if t.Failed() {
				outcomes[tg.name] = "FAIL"
			} else {
				outcomes[tg.name] = "PASS"
			}
			mu.Unlock()
		})
	}

	t.Cleanup(func() {
		mu.Lock()
		defer mu.Unlock()
		var b strings.Builder
		b.WriteString("\n==================== E2E MATRIX ====================\n")
		for _, tg := range targets {
			res := outcomes[tg.name]
			if res == "" {
				res = "UNKNOWN"
			}
			b.WriteString("  " + tg.name + ": " + res + "\n")
		}
		b.WriteString("===================================================\n")
		t.Log(b.String())
	})
}

func parseAllowlist(v string) map[string]bool {
	if v == "" {
		return nil
	}
	out := map[string]bool{}
	for _, name := range strings.Split(v, ",") {
		if n := strings.TrimSpace(name); n != "" {
			out[n] = true
		}
	}
	return out
}

// clusterHealthy reports whether last_seen_at is within window.
func clusterHealthy(lastSeen *time.Time, window time.Duration) (bool, string) {
	if lastSeen == nil {
		return false, "no last_seen_at"
	}
	age := time.Since(*lastSeen)
	if age > window {
		return false, "stale last_seen_at (" + age.Round(time.Second).String() + " > " + window.String() + ")"
	}
	return true, ""
}

func durationEnvOr(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

// sanitizeName makes a cluster name safe as part of a gateway name.
func sanitizeName(name string) string {
	r := strings.NewReplacer("/", "-", ".", "-", ":", "-", " ", "-", "_", "-")
	return strings.ToLower(r.Replace(name))
}

func managedKubeContextEnvKey(clusterName string) string {
	var b strings.Builder
	b.WriteString("E2E_MANAGED_KUBECONTEXT_")
	for _, r := range clusterName {
		if r >= 'a' && r <= 'z' {
			b.WriteRune(r - ('a' - 'A'))
		} else if r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}
