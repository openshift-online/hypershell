// Command fleet-dashboard is the read-only BFF for the HyperShell fleet "glass"
// dashboard. It aggregates Prometheus fleet metrics, GitOps Promoter + Argo
// promotion state, and topology ConfigMaps into a small cached JSON surface, and
// serves the embedded React SPA. See the operational-dashboard specs in the
// deployment (GitOps) repository for the full contract. No fleet-identifying
// values are compiled in; all arrive via FD_* environment variables
// (data-architecture spec §3.5).
package main

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/api"
	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/auth"
	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/cache"
	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/config"
	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/metrics"
	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/server"
	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/sources"
	"github.com/openshift-online/hypershell/components/fleet-dashboard/web"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger) // so package-level slog.Default() users (e.g. sources) share this handler
	if err := run(logger); err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	kube, err := sources.NewKubeClients(cfg)
	if err != nil {
		return err
	}

	prom := sources.NewPrometheus(cfg)
	// GitHub check-run enrichment is opt-in (FD_GITHUB_REPO): NewGitHubAnalysis
	// returns nil when disabled, which NewPromotion treats as the no-op resolver.
	// Assign through the interface var so a nil *GitHubAnalysis does not become a
	// non-nil interface holding a nil pointer.
	var analyzer sources.AnalysisResolver
	if gh := sources.NewGitHubAnalysis(cfg); gh != nil {
		analyzer = gh
		logger.Info("github analysis enrichment enabled", "repo", cfg.GitHubRepo)
	}
	// Release-bundle enrichment (dates + "in this bundle" PR lists) is opt-in on
	// the same FD_GITHUB_REPO: NewGitHubBundles returns nil when disabled, which
	// NewPromotion treats as the no-op enricher.
	var bundler sources.BundleEnricher
	if gb := sources.NewGitHubBundles(cfg); gb != nil {
		bundler = gb
		logger.Info("github bundle enrichment enabled", "repo", cfg.GitHubRepo)
	}
	// Resolve each environment's release by the versions-lock bundle it renders
	// (reference tag + digest) rather than the gitops dry-commit SHA, so envs on
	// the same bundle collapse and only a genuinely older bundle reads "behind".
	// Opt-in on the same FD_GITHUB_REPO; nil falls back to the short-SHA resolver.
	// The lock resolver doubles as the release-history source: it already caches
	// per-commit bundle resolutions, so walking the lock's commit history for the
	// previously-deployed freight cards reuses that cache.
	var versioner sources.VersionResolver
	var hydratedVersioner sources.HydratedVersionResolver
	var history sources.ReleaseHistory
	if lr := sources.NewGitHubLockResolver(cfg); lr != nil {
		versioner = lr
		hydratedVersioner = lr
		history = lr
		logger.Info("github release-lock version resolution enabled", "repo", cfg.GitHubRepo, "historyLimit", cfg.ReleaseHistoryLimit)
	}
	promotion := sources.NewPromotion(cfg, kube.Dynamic, versioner, analyzer, bundler, history)
	topology := sources.NewTopology(cfg, kube.Clientset)

	fleetSrc := cache.NewSource("fleet", cfg.RefreshFleet, prom.Fleet, metrics.Observe)
	promotionSrc := cache.NewSource("promotion", cfg.RefreshPromotion, promotion.Promotion, metrics.Observe)
	topologySrc := cache.NewSource("topology", cfg.RefreshTopology, topology.Topology, metrics.Observe)
	instancesSrc := cache.NewSource("instances", cfg.RefreshInstances, prom.Instances, metrics.Observe)

	// Agent SDLC funnel visibility (agent-sdlc-visibility-dashboard.spec). Funnel
	// state comes from the PUBLIC product repo through the GitHub-label adapter
	// behind the SdlcStateProvider port (a Jira adapter is a future drop-in). The
	// release-by-hub-cluster matrix reuses the SAME cached promotion bundle truth
	// (promotionSrc) plus one PR->closing-issue hop; it does not re-derive bundles.
	var sdlcProvider sources.SdlcStateProvider
	if gs := sources.NewGitHubSDLC(cfg); gs != nil {
		sdlcProvider = gs
		logger.Info("agent sdlc visibility enabled", "repo", cfg.UpstreamRepo, "refresh", cfg.RefreshSDLC)
	}
	var issueBundles sources.WorkItemPRResolver
	if ib := sources.NewGitHubWorkItemPRs(cfg); ib != nil {
		issueBundles = ib
	}
	var sdlcSrc *cache.Source
	if sdlcProvider != nil {
		sdlc := sources.NewSDLC(cfg, sdlcProvider, promotionSrc, issueBundles)
		sdlcSrc = cache.NewSource("sdlc", cfg.RefreshSDLC, sdlc.SDLC, metrics.Observe)
	}

	// E2E run/log visibility (e2e-run-log-visibility.spec). Opt-in on
	// FD_E2E_LOKI_URL: NewE2E returns nil when unconfigured, in which case the
	// recent-runs source is not started and the always-mounted e2e endpoints
	// report the feature off (E2E-LOG-01/02).
	var e2eSrc *cache.Source
	var e2ePlane api.E2ERunPlane
	if e2e := sources.NewE2E(cfg, hydratedVersioner); e2e != nil {
		e2eSrc = cache.NewSource("e2e", cfg.RefreshE2E, e2e.Runs, metrics.Observe)
		e2ePlane = e2e
		logger.Info("e2e run visibility enabled", "tenants", cfg.E2ELokiTenants, "window", cfg.E2EHistoryWindow, "limit", cfg.E2ERecentRunLimit)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	srcs := []*cache.Source{fleetSrc, promotionSrc, topologySrc, instancesSrc}
	if sdlcSrc != nil {
		srcs = append(srcs, sdlcSrc)
	}
	if e2eSrc != nil {
		srcs = append(srcs, e2eSrc)
	}
	for _, s := range srcs {
		go s.Run(ctx)
	}

	handlers := &api.Handlers{
		Fleet:     fleetSrc,
		Promotion: promotionSrc,
		Topology:  topologySrc,
		Instances: instancesSrc,
		SDLC:      sdlcSrc,
		E2ERuns:   e2eSrc,
		E2E:       e2ePlane,
	}
	authn := auth.New(cfg, kube.Clientset)

	static, err := staticFS(cfg)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           server.New(handlers, authn, static),
		ReadHeaderTimeout: 10 * time.Second,
	}
	// Dedicated /metrics listener, not fronted by the oauth-proxy, scraped
	// directly by Prometheus (data-architecture.spec §9).
	metricsSrv := &http.Server{
		Addr:              cfg.MetricsAddr,
		Handler:           server.Metrics(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", cfg.ListenAddr, "authEnabled", cfg.AuthEnabled)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	go func() {
		logger.Info("metrics listening", "addr", cfg.MetricsAddr)
		if err := metricsSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutting down")
	case err := <-errCh:
		return err
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = metricsSrv.Shutdown(shutdownCtx)
	return srv.Shutdown(shutdownCtx)
}

// staticFS serves the UI from an on-disk directory when FD_STATIC_ROOT is set
// (local dev against a Vite build), otherwise from the embedded bundle.
func staticFS(cfg *config.Config) (fs.FS, error) {
	if cfg.StaticRoot != "" {
		return os.DirFS(cfg.StaticRoot), nil
	}
	return web.FS()
}
