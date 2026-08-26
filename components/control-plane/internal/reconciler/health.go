package reconciler

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/exposure"
	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
	"github.com/openshift-online/hypershell/components/control-plane/internal/keycloak"
	cpotel "github.com/openshift-online/hypershell/components/control-plane/internal/otel"
	"google.golang.org/grpc"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// defaultHealthInterval is the cadence at which the control plane observes
// gateway workload health and synchronizes the Gateway phase.
const defaultHealthInterval = 30 * time.Second

// defaultRouteReadyTimeout is the grace window a routed gateway's external
// exposure may remain not-Ready (after its Deployment is Ready) before the
// control plane moves the gateway to Degraded. See
// specs/platform/openshell-gateway-routing.spec.md § Gateway Exposure Configuration.
const defaultRouteReadyTimeout = 10 * time.Minute

// defaultListGatewaysPageSize is the number of gateways requested per page
// when retrieving the full gateway fleet for health observation.
const defaultListGatewaysPageSize = 100

// routeVerifyInterval is the minimum time between residual route/console
// absence re-checks for a settled (torn-down, addressless) gateway.
//
// Verification is deliberately NOT bounded to a fixed window after teardown.
// A stale in-flight provisioning pass creates the GRPCRoute before its
// TLS-secret wait and fail-closed route-intent re-check, so elapsed wall-clock
// time is not proof that every stale writer has drained -- a late pass can
// resurrect resources after any wall-clock window would have expired. Instead
// the health loop keeps re-verifying absence indefinitely, but at most once per
// interval, so a settled non-routed gateway costs one cheap absence probe per
// interval (not per tick). That bounds steady-state Keycloak/apiserver traffic
// at fleet scale while never trusting a wall-clock guess that resources stay
// gone. Comfortably larger than the provisioning path's 60s TLS wait plus
// reconcile time, so the steady-state cost is low.
const routeVerifyInterval = 5 * time.Minute

// GatewayHealthReconciler continuously observes the health of provisioned
// gateway Deployments and, for routed gateways, the readiness of their external
// exposure, and keeps each Gateway's `phase` and `status` synchronized with
// actual state. It runs independently of the provisioning phase gate: a Running
// gateway whose pod begins crash-looping (or whose route loses readiness) is
// moved to Degraded, and a Degraded gateway whose workload and exposure recover
// is moved back to Running. See openshell-gateway-health.spec.md.
type GatewayHealthReconciler struct {
	clientset           *kubernetes.Clientset
	dynamicClient       dynamic.Interface
	grpcConn            *grpc.ClientConn
	interval            time.Duration
	exposure            exposure.Port
	routeReadyTimeout   time.Duration
	keycloakConfig      *gateway.KeycloakConfig
	isOpenShift         bool
	skipNetworkPolicies bool

	// consoleClientChecker is a single, long-lived Keycloak client reused across
	// every tick's residual-absence checks. Constructed once (when Keycloak is
	// configured) so its token cache is preserved: a fresh client per check would
	// perform a client-credentials token request on every settled gateway every
	// tick, fleet-amplifying admin authentication in the serial health loop. Nil
	// when Keycloak is unconfigured. The health loop is serial, so a single shared
	// client needs no additional synchronization.
	consoleClientChecker gateway.ConsoleClientChecker

	// now is the clock, overridable in tests.
	now func() time.Time

	// routeNotReadySince records, per gateway, when its Deployment first became
	// Ready while its external exposure was not, so the route-readiness grace
	// window can be enforced during provisioning. Entries are cleared once the
	// gateway settles (Running, Degraded, or Deployment not Ready). In-memory
	// only: on restart the window restarts, which is acceptable.
	//
	// routeTornDown records, per gateway, that a full route+console teardown has
	// completed with no residual resources or stored addresses, so subsequent
	// ticks skip the (otherwise per-tick) delete and Keycloak traffic. It is set
	// only on a clean teardown and cleared the moment the gateway is routed again,
	// so a teardown that failed part-way keeps retrying until everything is gone.
	// In-memory only: on restart a single confirming teardown pass re-runs, which
	// is acceptable.
	//
	// routeVerifiedAt records, per gateway, when residual route/console absence
	// was last confirmed, so the (indefinite) re-verification runs at most once
	// per routeVerifyInterval rather than every tick. It is not a cutoff: a
	// settled gateway keeps being re-verified forever at that low cadence, because
	// elapsed wall-clock time is not proof that a stale provisioning pass cannot
	// still resurrect resources (see routeVerifyInterval).
	mu                 sync.Mutex
	routeNotReadySince map[string]time.Time
	routeTornDown      map[string]bool
	routeVerifiedAt    map[string]time.Time
}

func NewGatewayHealthReconciler(clientset *kubernetes.Clientset, dynamicClient dynamic.Interface, grpcConn *grpc.ClientConn, exposurePort exposure.Port, keycloakConfig *gateway.KeycloakConfig) *GatewayHealthReconciler {
	// Build one long-lived Keycloak client for residual-absence checks so its
	// token cache survives across ticks (see consoleClientChecker).
	var consoleClientChecker gateway.ConsoleClientChecker
	if keycloakConfig != nil {
		consoleClientChecker = keycloak.NewClient(
			keycloakConfig.ServerURL,
			keycloakConfig.Realm,
			keycloakConfig.ClientID,
			keycloakConfig.ClientSecret,
		)
	}
	// Mirror GatewayReconciler's environment detection so the health loop's
	// console self-heal produces the same resources the provisioning path would.
	return &GatewayHealthReconciler{
		clientset:            clientset,
		dynamicClient:        dynamicClient,
		grpcConn:             grpcConn,
		interval:             defaultHealthInterval,
		exposure:             exposurePort,
		routeReadyTimeout:    routeReadyTimeout(),
		keycloakConfig:       keycloakConfig,
		consoleClientChecker: consoleClientChecker,
		isOpenShift:          gateway.DetectOpenShift(clientset),
		skipNetworkPolicies:  os.Getenv("GATEWAY_SKIP_NETWORK_POLICIES") == "true",
		now:                  time.Now,
		routeNotReadySince:   make(map[string]time.Time),
		routeTornDown:        make(map[string]bool),
		routeVerifiedAt:      make(map[string]time.Time),
	}
}

// routeReadyTimeout resolves the route-readiness grace window from
// GATEWAY_ROUTE_READY_TIMEOUT, falling back to the default.
func routeReadyTimeout() time.Duration {
	if v := os.Getenv("GATEWAY_ROUTE_READY_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
		log.Printf("WARN invalid GATEWAY_ROUTE_READY_TIMEOUT %q; using default %s", v, defaultRouteReadyTimeout)
	}
	return defaultRouteReadyTimeout
}

// Run drives the health reconciliation loop until the context is cancelled.
func (h *GatewayHealthReconciler) Run(ctx context.Context) error {
	log.Printf("INFO gateway health reconciler started (interval=%s routeReadyTimeout=%s)", h.interval, h.routeReadyTimeout)
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			h.reconcileOnce(ctx)
		}
	}
}

func (h *GatewayHealthReconciler) reconcileOnce(ctx context.Context) {
	ctx, endSpan := cpotel.StartReconcileSpan(ctx, "gateway-health", "reconcile")
	var tickErr error
	defer func() { endSpan(tickErr) }()

	client := pb.NewGatewayServiceClient(h.grpcConn)
	// Page through the whole fleet: the list endpoint is server-side paginated
	// (default page size 20), so an unpaged request would only ever refresh the
	// health of the first page of gateways.
	gateways, err := h.listAllGateways(ctx, client)
	if err != nil {
		tickErr = err
		log.Printf("WARN gateway health: list gateways: %v", err)
		return
	}

	for _, gw := range gateways {
		h.reconcileGatewayHealth(ctx, client, gw)
	}
}

// listAllGateways retrieves all gateways from the API server across all pages.
func (h *GatewayHealthReconciler) listAllGateways(ctx context.Context, client pb.GatewayServiceClient) ([]*pb.Gateway, error) {
	var all []*pb.Gateway
	page := int32(1)

	for {
		resp, err := client.ListGateways(ctx, &pb.ListGatewaysRequest{
			Page: page,
			Size: defaultListGatewaysPageSize,
		})
		if err != nil {
			return nil, err
		}

		items := resp.GetItems()
		all = append(all, items...)

		meta := resp.GetMetadata()
		if len(items) == 0 || (meta != nil && int64(len(all)) >= int64(meta.GetTotal())) || len(items) < int(defaultListGatewaysPageSize) {
			break
		}
		page++
	}

	return all, nil
}

func (h *GatewayHealthReconciler) reconcileGatewayHealth(ctx context.Context, client pb.GatewayServiceClient, gw *pb.Gateway) {
	gatewayID := gw.GetMetadata().GetId()
	if gatewayID == "" {
		return
	}
	phase := gw.GetPhase()

	// Only gateways the provisioning path has already acted upon carry an
	// observable workload. Leave Pending gateways to the provisioning path and
	// Failed gateways to a subsequent spec change.
	switch phase {
	case "Running", "Degraded", "Provisioning":
	default:
		return
	}

	// Keep the console_address in sync with the console pod's readiness so the web
	// UI's console button only appears once the console can serve (and disappears
	// if it later goes unready). Independent of the gateway workload's own phase.
	consoleServable := syncConsoleAddress(ctx, h.clientset, h.dynamicClient, client, gatewayID, gw, h.exposure != nil)

	// Self-heal the console. A console failure is deliberately non-fatal to the
	// gateway, so once the gateway reaches Running the provisioning path never runs
	// again and a transient console failure -- or later drift, e.g. a deleted
	// console HTTPRoute or Deployment -- would otherwise never be retried. Re-run
	// the (idempotent) console reconcile here only when the console is observed not
	// servable, so a healthy console adds no steady-state Keycloak or apply
	// traffic. Errors are logged and never affect the gateway phase.
	if !consoleServable {
		h.selfHealConsole(ctx, gatewayID, gw)
	}

	// Reconcile the route's desired absence. A gateway whose route was removed
	// keeps its route resources (GRPCRoute, BackendTLSPolicy, backend-CA
	// ConfigMap, router NetworkPolicy) and its console (Deployment, Service,
	// HTTPRoute, Keycloak client), plus the published route_address and
	// console_address, until torn down. syncConsoleAddress and selfHealConsole
	// both no-op for a non-routed gateway, and the provisioning path never runs
	// again for a gateway the health loop owns (phase gate), so this is the only
	// place an un-routed gateway's route and console are cleaned up. Clearing the
	// torn-down marker while routed lets a later un-routing trigger a fresh
	// teardown.
	if isRoutedGateway(gw) {
		h.clearRouteTornDown(gatewayID)
	} else {
		h.teardownRoute(ctx, client, gatewayID, gw)
	}

	namespace, err := gatewayNamespace(gw)
	if err != nil {
		log.Printf("WARN gateway health: %s: %v", gatewayID, err)
		return
	}
	ready, reason, err := gateway.DeploymentReadiness(ctx, h.clientset, namespace, gateway.GatewayDeploymentName)
	if err != nil {
		log.Printf("WARN gateway health: %s: %v", gatewayID, err)
		return
	}

	var desiredPhase, desiredStatus string
	switch {
	case !ready:
		// The Deployment has not been created yet; the provisioning path still
		// owns this gateway. Leave its phase untouched.
		if reason == "deployment not found" {
			return
		}
		h.clearRouteTimer(gatewayID)
		desiredPhase, desiredStatus = "Degraded", reason
	case h.exposure != nil && isRoutedGateway(gw):
		// Deployment is Ready; a routed gateway additionally requires its external
		// exposure to be observed Ready before it can be Running.
		desiredPhase, desiredStatus = h.evaluateRouteReadiness(ctx, gatewayID, namespace, phase)
		if desiredPhase == "" {
			// Transient error observing the exposure; leave the phase untouched
			// rather than flap the gateway.
			return
		}
	default:
		h.clearRouteTimer(gatewayID)
		desiredPhase, desiredStatus = "Running", "Healthy"
	}

	// active_sandbox_count is maintained independently by the event-driven
	// sandbox-count reconciler (see openshell-gateway-sandbox-count.spec.md); the
	// health reconciler only owns phase and status.
	if phase == desiredPhase && gw.GetStatus() == desiredStatus {
		return
	}

	if _, err := client.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:     gatewayID,
		Phase:  &desiredPhase,
		Status: &desiredStatus,
	}); err != nil {
		log.Printf("WARN gateway health: update %s to %s: %v", gatewayID, desiredPhase, err)
		return
	}

	log.Printf("INFO gateway health: %s %s -> %s (%s)", gatewayID, phase, desiredPhase, desiredStatus)
}

// selfHealConsole re-reconciles the per-gateway console when it is observed not
// servable, so a console that failed to provision or has since drifted is
// recreated without a gateway spec change. It is a no-op unless the gateway is
// routed (its console lifecycle follows the route) and Keycloak is configured.
// The reconcile is idempotent and its failures are logged, never propagated:
// they must not perturb the gateway's own health phase.
func (h *GatewayHealthReconciler) selfHealConsole(ctx context.Context, gatewayID string, gw *pb.Gateway) {
	if h.exposure == nil || !isRoutedGateway(gw) || h.keycloakConfig == nil {
		return
	}
	namespace, err := gatewayNamespace(gw)
	if err != nil {
		log.Printf("WARN console self-heal for %s: %v", gatewayID, err)
		return
	}
	opts := gateway.ReconcileOpts{
		IsOpenShift:         h.isOpenShift,
		SkipNetworkPolicies: h.skipNetworkPolicies,
		Keycloak:            h.keycloakConfig,
		GatewayID:           gatewayID,
		GatewayName:         gw.GetName(),
	}
	if err := gateway.ReconcileConsole(ctx, h.dynamicClient, h.clientset, gateway.NamespaceConfig{Name: namespace}, opts); err != nil {
		log.Printf("WARN console self-heal in %s: %v", namespace, err)
		return
	}
	log.Printf("INFO console self-heal reconciled in %s", namespace)
}

// teardownRoute reconciles the desired absence of the route -- and the console
// that follows it -- for a gateway the health loop owns that is no longer routed,
// mirroring the provisioning path's route-disabled branch (which the phase gate
// prevents from running again once the gateway is Running). It removes the
// GRPCRoute, BackendTLSPolicy, backend-CA ConfigMap, router NetworkPolicy and all
// console resources + Keycloak client, and clears the published route_address and
// console_address.
//
// It keeps retrying every tick until DeleteGatewayAPIResources reports a clean
// pass (no residual resources, no address left to clear), then records the
// gateway as torn down so subsequent ticks add no delete or Keycloak traffic. A
// teardown that fails part-way is deliberately left unmarked so the next tick
// retries -- teardown must converge on full absence, not stop on partial cleanup.
// Failures are logged, never propagated: route teardown must not perturb the
// gateway's own health phase.
func (h *GatewayHealthReconciler) teardownRoute(ctx context.Context, client pb.GatewayServiceClient, gatewayID string, gw *pb.Gateway) {
	namespace, err := gatewayNamespace(gw)
	if err != nil {
		log.Printf("WARN route teardown for %s: %v", gatewayID, err)
		return
	}
	// A prior clean teardown lets us skip the (expensive) per-tick delete and
	// Keycloak traffic -- but the marker is only a cache and cleared address
	// fields do not prove the resources are gone. A stale provisioning pass can
	// recreate the route/console resources after a teardown believed itself
	// complete (the post-TLS-wait re-check narrows but cannot fully close that
	// window). So before trusting the marker, verify the owned resources are
	// actually absent; if any reappeared -- or absence cannot be confirmed --
	// drop the marker and re-run teardown so cleanup converges on real absence.
	if h.teardownSettled(gatewayID, gw) {
		// Re-verify absence at a low, indefinite cadence rather than only within a
		// wall-clock window after teardown: a stale provisioning pass can create the
		// GRPCRoute before its TLS wait and fail-closed route-intent re-check, so no
		// elapsed time proves the resurrection race has drained. Between verifications
		// trust the completion marker so a settled non-routed gateway costs at most one
		// probe per routeVerifyInterval -- not one per tick -- at fleet scale.
		if !h.dueForVerify(gatewayID) {
			return
		}
		// Verify actual absence. Include the external Keycloak console client (a realm
		// object a Kubernetes-only probe cannot see) via the single long-lived checker
		// so its token cache is reused. If any resource reappeared -- or absence cannot
		// be confirmed -- drop the marker and re-run teardown so cleanup converges on
		// real absence; if confirmed absent, stamp the verification so the next probe is
		// one interval away.
		consoleClientID := ""
		if h.consoleClientChecker != nil && gw.GetName() != "" && gatewayID != "" {
			consoleClientID = fmt.Sprintf("%s-%s-console", gw.GetName(), gatewayID)
		}
		absent, perr := gateway.RouteResourcesAbsent(ctx, h.dynamicClient, h.clientset, namespace, h.consoleClientChecker, consoleClientID)
		switch {
		case perr != nil:
			// Leave the verification timestamp stale so the next tick re-probes rather
			// than waiting a full interval, and re-run teardown now in case resources
			// reappeared while the probe was failing.
			log.Printf("WARN route teardown: cannot confirm resource absence in %s; re-running teardown: %v", namespace, perr)
			h.clearRouteTornDown(gatewayID)
		case absent:
			h.markVerified(gatewayID)
			return
		default:
			log.Printf("INFO route/console resources reappeared in %s (stale provision after teardown); re-running teardown", namespace)
			h.clearRouteTornDown(gatewayID)
		}
	}
	opts := gateway.ReconcileOpts{
		IsOpenShift:         h.isOpenShift,
		SkipNetworkPolicies: h.skipNetworkPolicies,
		Keycloak:            h.keycloakConfig,
		GatewayID:           gatewayID,
		GatewayName:         gw.GetName(),
	}
	// Wire an address-clearing callback only when an address is actually stored,
	// so a gateway that never published one adds no gateway-update traffic.
	if gw.GetRouteAddress() != "" {
		opts.UpdateRouteAddress = func(ctx context.Context, routeAddress string) error {
			_, uerr := client.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
				Id:           gatewayID,
				RouteAddress: &routeAddress,
			})
			return uerr
		}
	}
	if gw.GetConsoleAddress() != "" {
		opts.UpdateConsoleAddress = func(ctx context.Context, consoleAddress string) error {
			_, uerr := client.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
				Id:             gatewayID,
				ConsoleAddress: &consoleAddress,
			})
			return uerr
		}
	}
	if err := gateway.DeleteGatewayAPIResources(ctx, h.dynamicClient, h.clientset, namespace, opts); err != nil {
		// Leave the gateway unmarked so the next tick retries until every
		// route-owned resource and stored address is gone.
		log.Printf("WARN route teardown in %s (gateway no longer routed): %v", namespace, err)
		return
	}
	h.markRouteTornDown(gatewayID)
	log.Printf("INFO route and console torn down in %s (gateway no longer routed)", namespace)
}

// teardownSettled is the cheap first gate teardownRoute consults before doing
// the more expensive owned-resource absence probe: it reports whether the
// completion marker is set AND the Gateway record carries no route or console
// address. Requiring both stored addresses to be empty defends against a late
// address write -- the console-address publisher started during provisioning
// runs on the long-lived watch context that route removal does not cancel, so it
// can write console_address after a teardown marked itself complete; the moment
// one address reappears the marker is untrusted and teardown re-runs to clear it.
// A true result here does NOT by itself authorize skipping teardown: the caller
// still verifies actual resource absence (RouteResourcesAbsent), because empty
// address fields do not prove a stale provisioning pass hasn't recreated the
// route/console resources. Teardown must converge on full absence of resources
// and stored addresses, never stop on a stale cache.
func (h *GatewayHealthReconciler) teardownSettled(gatewayID string, gw *pb.Gateway) bool {
	return h.routeTornDownAlready(gatewayID) && gw.GetRouteAddress() == "" && gw.GetConsoleAddress() == ""
}

// routeTornDownAlready reports whether a clean route+console teardown has already
// completed for the gateway.
func (h *GatewayHealthReconciler) routeTornDownAlready(gatewayID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.routeTornDown[gatewayID]
}

// markRouteTornDown records that the gateway's route and console are fully absent,
// stamping the time as the baseline for the low-frequency re-verification so the
// first re-check happens one routeVerifyInterval later.
func (h *GatewayHealthReconciler) markRouteTornDown(gatewayID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.routeTornDown[gatewayID] = true
	if h.routeVerifiedAt == nil {
		h.routeVerifiedAt = make(map[string]time.Time)
	}
	h.routeVerifiedAt[gatewayID] = h.now()
}

// markVerified stamps a successful residual-absence verification so the next probe
// is one routeVerifyInterval away.
func (h *GatewayHealthReconciler) markVerified(gatewayID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.routeVerifiedAt == nil {
		h.routeVerifiedAt = make(map[string]time.Time)
	}
	h.routeVerifiedAt[gatewayID] = h.now()
}

// clearRouteTornDown forgets any recorded teardown for a gateway, so a later
// un-routing triggers a fresh teardown.
func (h *GatewayHealthReconciler) clearRouteTornDown(gatewayID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.routeTornDown, gatewayID)
	delete(h.routeVerifiedAt, gatewayID)
}

// dueForVerify reports whether the settled gateway's residual-absence has not been
// verified within routeVerifyInterval, so it is worth probing again this tick.
// Verification is indefinite (never a wall-clock cutoff): a settled gateway is
// always re-verified eventually, just at most once per interval.
func (h *GatewayHealthReconciler) dueForVerify(gatewayID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	at, ok := h.routeVerifiedAt[gatewayID]
	if !ok {
		// No recorded verification (e.g. a marker set before this field existed, or a
		// restart): verify rather than trusting an unstamped marker.
		return true
	}
	return h.now().Sub(at) >= routeVerifyInterval
}

// evaluateRouteReadiness decides the phase for a routed gateway whose Deployment
// is Ready, based on its external-exposure readiness. It returns an empty phase
// to signal "leave untouched" when the exposure cannot be observed.
//
// The route-readiness grace window applies only while the gateway is still
// provisioning (never yet Running): within the window it stays Provisioning,
// beyond it becomes Degraded. A gateway that had reached Running and then loses
// readiness is moved to Degraded immediately, and a Degraded gateway stays
// Degraded until its exposure recovers.
func (h *GatewayHealthReconciler) evaluateRouteReadiness(ctx context.Context, gatewayID, namespace, currentPhase string) (string, string) {
	rr, err := h.exposure.ObserveReadiness(ctx, exposure.Request{Namespace: namespace})
	if err != nil {
		log.Printf("WARN gateway health: observe exposure for %s: %v", gatewayID, err)
		return "", ""
	}
	if rr.Ready {
		h.clearRouteTimer(gatewayID)
		return "Running", "Healthy"
	}

	if currentPhase == "Provisioning" {
		since := h.markRouteNotReady(gatewayID)
		if h.now().Sub(since) >= h.routeReadyTimeout {
			h.clearRouteTimer(gatewayID)
			return "Degraded", fmt.Sprintf("route not ready after %s: %s", h.routeReadyTimeout, rr.Reason)
		}
		return "Provisioning", rr.Reason
	}

	// currentPhase is Running (lost readiness) or Degraded (still unhealthy).
	h.clearRouteTimer(gatewayID)
	return "Degraded", rr.Reason
}

// markRouteNotReady records the first time the gateway's Deployment was observed
// Ready while its route was not, returning that timestamp (existing or now).
func (h *GatewayHealthReconciler) markRouteNotReady(gatewayID string) time.Time {
	h.mu.Lock()
	defer h.mu.Unlock()
	if t, ok := h.routeNotReadySince[gatewayID]; ok {
		return t
	}
	t := h.now()
	h.routeNotReadySince[gatewayID] = t
	return t
}

// clearRouteTimer forgets any recorded route-not-ready start time for a gateway.
func (h *GatewayHealthReconciler) clearRouteTimer(gatewayID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.routeNotReadySince, gatewayID)
}
