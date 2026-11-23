package prober

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Prober runs the probe loops and the metrics endpoint (OBS-PRB-10). It holds no
// reconciler state and dials every target only through its public endpoint
// (OBS-PRB-01).
type Prober struct {
	cfg     *Config
	metrics *Metrics

	// apiClient lists gateways and managed clusters to build the gateway health
	// target set (OBS-PRB-02); nil when no gateway API URL is configured.
	apiClient *APIClient
	// auth acquires the keyless HyperShell API token in the configured mode
	// (OBS-PRB-06); nil when the selected mode is not configured.
	auth authenticator
	// dial opens a gateway health connection (OBS-PRB-03).
	dial healthDialer
	// httpClient issues the API read probes (OBS-PRB-04, OBS-PRB-05); the
	// per-probe timeout is applied through the request context, not this client.
	httpClient *http.Client

	// seenGateways is the set of gateway ids with a live metric series. It is
	// touched only by the single gateway health loop goroutine, so it needs no
	// lock. It lets each cycle prune series for gateways that are no longer
	// eligible, keeping gateway_id cardinality bounded by fleet size (OBS-PRB-07).
	seenGateways map[string]struct{}
}

// New builds a Prober from configuration. It wires the keyless identity, the API
// client, the gateway dialer, and the metrics registry. It performs no network
// I/O and never depends on reconciler-only configuration, so the probe mode
// starts even when the gateway database admin credentials are absent
// (OBS-PRB-10.02).
func New(cfg *Config) *Prober {
	auth := cfg.Authenticator()

	// Pass a genuinely nil tokenSource (not a non-nil interface wrapping a nil
	// pointer) when there is no authenticator, so the API client omits the bearer
	// header rather than panicking.
	var tokens tokenSource
	if auth != nil {
		tokens = auth
	}

	var apiClient *APIClient
	if cfg.GatewayAPIURL != "" {
		apiClient = NewAPIClient(cfg.GatewayAPIURL, tokens, cfg.Timeout)
	}

	return &Prober{
		cfg:          cfg,
		metrics:      NewMetrics(),
		apiClient:    apiClient,
		auth:         auth,
		dial:         newTLSHealthDialer(nil),
		httpClient:   &http.Client{},
		seenGateways: map[string]struct{}{},
	}
}

// Run starts the metrics endpoint and the probe loops and blocks until ctx is
// cancelled (OBS-PRB-10.01). It runs only the prober: no reconciler, no gRPC
// watch streams, no gateway database provisioning, and no Helm gateway management.
func (p *Prober) Run(ctx context.Context) error {
	slog.InfoContext(ctx, "Outside prober starting", "config", p.cfg.Summary())

	// Derive a cancelable context so a fatal metrics-endpoint failure (for example
	// a bind failure) tears the whole prober down loudly and the pod restarts,
	// rather than running on silently with no scrape surface (OBS-PRB-07).
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	var serveErr error

	wg.Go(func() {
		if err := p.metrics.Serve(ctx, p.cfg.MetricsAddr, p.cfg.MetricsPath); err != nil {
			serveErr = err
			slog.ErrorContext(ctx, "Prober metrics endpoint failed", "error", err)
			cancel()
		}
	})

	if p.apiClient != nil {
		wg.Go(func() { p.runLoop(ctx, "gateway-health", p.gatewayHealthCycle) })
	} else {
		slog.WarnContext(ctx, "Gateway health probe disabled: no gateway API URL configured (HYPERSHELL_PROBE_GATEWAY_API_URL)")
	}

	if len(p.cfg.ProductionInstances) > 0 {
		wg.Go(func() { p.runLoop(ctx, "api-availability", p.apiReadCycle) })
		wg.Go(func() { p.runLoop(ctx, "api-authentication", p.apiAuthCycle) })
	} else {
		slog.WarnContext(ctx, "API availability and authentication probes disabled: no production instances configured (HYPERSHELL_PROBE_API_INSTANCES)")
	}

	<-ctx.Done()
	slog.InfoContext(ctx, "Prober shutdown signal received, waiting for loops to stop")
	wg.Wait()
	slog.InfoContext(ctx, "Outside prober stopped")
	// Surface a metrics-endpoint failure as a non-nil error so the process exits
	// non-zero; an ordinary shutdown (parent ctx cancelled) returns nil.
	if serveErr != nil {
		return serveErr
	}
	return nil
}

// runLoop runs cycle immediately and then every probe interval until ctx is
// cancelled. Running immediately means a newly started prober emits its first
// results without waiting a full interval, and the fixed cadence keeps a newly
// started outage inside the one-minute budget (OBS-PRB-09).
func (p *Prober) runLoop(ctx context.Context, name string, cycle func(context.Context)) {
	cycle(ctx)
	ticker := time.NewTicker(p.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.InfoContext(ctx, "Prober loop stopping", "loop", name)
			return
		case <-ticker.C:
			cycle(ctx)
		}
	}
}

// gatewayHealthCycle runs one gateway health probe cycle (OBS-PRB-03): it lists
// the fleet, derives the eligible targets (OBS-PRB-02), and probes them
// concurrently. Listing fails closed (the cycle is skipped and no result is
// synthesized) because the target set is unknown; a managed-cluster list failure
// degrades only attribution to "unknown".
func (p *Prober) gatewayHealthCycle(ctx context.Context) {
	listCtx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
	gateways, err := p.apiClient.ListGateways(listCtx)
	cancel()
	if err != nil {
		slog.WarnContext(ctx, "Prober could not list gateways, skipping gateway health cycle", "error", err)
		return
	}

	clustersCtx, cancelClusters := context.WithTimeout(ctx, p.cfg.Timeout)
	clusters, err := p.apiClient.ListManagedClusters(clustersCtx)
	cancelClusters()
	if err != nil {
		slog.WarnContext(ctx, "Prober could not list managed clusters, attributing gateways to \"unknown\" this cycle", "error", err)
		clusters = nil
	}

	targets := EligibleGateways(gateways, clusters, time.Now(), p.cfg.GraceWindow)

	// Prune series for gateways that are no longer eligible so the gateway_id
	// cardinality stays bounded by the current fleet size, not cumulative churn
	// (OBS-PRB-07).
	current := make(map[string]struct{}, len(targets))
	for _, t := range targets {
		current[t.GatewayID] = struct{}{}
	}
	for id := range p.seenGateways {
		if _, stillEligible := current[id]; !stillEligible {
			p.metrics.DeleteGatewaySeries(id)
		}
	}
	p.seenGateways = current

	probeAll(ctx, targets, p.cfg.Workers, func(ctx context.Context, t GatewayTarget) {
		probeCtx, probeCancel := context.WithTimeout(ctx, p.cfg.Timeout)
		defer probeCancel()
		result := probeGatewayHealth(probeCtx, p.dial, t.RouteAddress)
		p.metrics.RecordGatewayHealth(t.GatewayID, t.ManagedCluster, result)
	})
}

// apiReadCycle runs one API availability probe cycle (OBS-PRB-04) across every
// production instance concurrently.
func (p *Prober) apiReadCycle(ctx context.Context) {
	probeAll(ctx, p.cfg.ProductionInstances, p.cfg.Workers, func(ctx context.Context, instance string) {
		probeCtx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
		defer cancel()
		token := p.bestEffortToken(probeCtx)
		result := probeAPIAvailability(probeCtx, p.httpClient, instance, p.cfg.APIReadPath, token)
		p.metrics.RecordAPIRead(instance, result)
	})
}

// apiAuthCycle runs one API authentication probe cycle (OBS-PRB-05) across every
// production instance concurrently.
func (p *Prober) apiAuthCycle(ctx context.Context) {
	probeAll(ctx, p.cfg.ProductionInstances, p.cfg.Workers, func(ctx context.Context, instance string) {
		probeCtx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
		defer cancel()
		result := probeAPIAuth(probeCtx, p.httpClient, instance, p.cfg.APIReadPath, p.exchangeFn())
		p.metrics.RecordAPIAuth(instance, result)
	})
}

// bestEffortToken returns a cached access token for the API availability probe, or
// the empty string when none is available. The availability probe must not fail
// because authentication is broken (that is OBS-PRB-05's concern), so a token
// error is swallowed and the read proceeds unauthenticated.
func (p *Prober) bestEffortToken(ctx context.Context) string {
	if p.auth == nil {
		return ""
	}
	token, err := p.auth.Token(ctx)
	if err != nil {
		return ""
	}
	return token
}

// exchangeFn returns the token acquisition step for the authentication probe. When
// no authentication mode is configured, it returns an error so the authentication
// probe is recorded as failed (OBS-PRB-05.02) rather than silently skipped. When it
// is, it forces a fresh token acquisition each cycle so the probe genuinely
// exercises Keycloak (OBS-PRB-06).
func (p *Prober) exchangeFn() func(context.Context) (string, error) {
	if p.auth == nil {
		return func(context.Context) (string, error) {
			return "", errors.New("prober authentication not configured")
		}
	}
	return p.auth.ExchangeFresh
}

// probeAll runs fn over items with a bounded worker pool (OBS-PRB-08): at most
// workers probes run at once, and a single slow target occupies only its own
// worker until its per-probe timeout, so it cannot delay the others beyond the
// probe interval. It returns when every probe has completed or ctx is cancelled.
func probeAll[T any](ctx context.Context, items []T, workers int, fn func(context.Context, T)) {
	if workers < 1 {
		workers = 1
	}
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for _, item := range items {
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(it T) {
			defer wg.Done()
			defer func() { <-sem }()
			fn(ctx, it)
		}(item)
	}
	wg.Wait()
}
