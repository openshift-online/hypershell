package reconciler

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/gatewayhealth"
	"github.com/openshift-online/hypershell/components/control-plane/internal/exposure"
	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
	cpotel "github.com/openshift-online/hypershell/components/control-plane/internal/otel"
	"google.golang.org/grpc"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// defaultHealthInterval is the cadence at which the control plane observes
// gateway workload health and synchronizes the Gateway phase.
const defaultHealthInterval = 30 * time.Second

// gatewayHealthWorkerCount limits concurrent gateway health passes.
const gatewayHealthWorkerCount = 4

// defaultRouteReadyTimeout is the grace window a routed gateway's external
// exposure may remain not-Ready (after its Deployment is Ready) before the
// control plane moves the gateway to Degraded. See
// specs/platform/openshell-gateway-routing.spec.md § Gateway Exposure Configuration.
const defaultRouteReadyTimeout = 10 * time.Minute

// defaultDeploymentReadyTimeout is the grace window a provisioning gateway's
// Deployment may remain not-Ready (pods pending, pulling images, or starting)
// before the health loop moves the gateway to Degraded. The provisioning path
// normally reports Degraded itself once its own, shorter readiness window
// elapses; this window is the health loop's backstop for a provisioning pass
// that never completes (e.g. the control plane restarted mid-provision). See
// openshell-gateway-health.spec.md § Phase Reflects Workload and Route Readiness.
const defaultDeploymentReadyTimeout = 10 * time.Minute

// GatewayHealthReconciler continuously observes the health of provisioned
// gateway Deployments and, for routed gateways, the readiness of their external
// exposure, and keeps each Gateway's `phase` and `status` synchronized with
// actual state. It runs independently of the provisioning phase gate: a Running
// gateway whose pod begins crash-looping (or whose route loses readiness) is
// moved to Degraded, and a Degraded gateway whose workload and exposure recover
// is moved back to Running. See openshell-gateway-health.spec.md.
type GatewayHealthReconciler struct {
	// clientset is the interface type (not the concrete *kubernetes.Clientset) so
	// health passes can be driven by a fake in tests. Production wires the real
	// clientset via NewGatewayHealthReconciler.
	clientset     kubernetes.Interface
	dynamicClient dynamic.Interface
	grpcConn      *grpc.ClientConn
	// vaultReachable probes whether a vault-backed gateway's provider Vault is
	// reachable. A nil value uses gateway.ProbeVaultReachable; tests inject a stub.
	vaultReachable func(ctx context.Context, clientset kubernetes.Interface) error
	// clusterID scopes the health sweep to this managed cluster's gateways. The
	// list is filtered server-side so a control plane never stamps
	// (Degraded/Running) a gateway owned by another cluster. Always set: it is
	// the control plane's registered cluster id.
	clusterID              string
	interval               time.Duration
	exposure               exposure.Port
	routeReadyTimeout      time.Duration
	deploymentReadyTimeout time.Duration
	keycloakConfig         *gateway.KeycloakConfig
	isOpenShift            bool
	versionObserver        gatewayVersionObserver
	controlPlaneNamespace  string

	// now is the clock, overridable in tests.
	now func() time.Time

	// routeNotReadySince records, per gateway, when its Deployment first became
	// Ready while its external exposure was not, so the route-readiness grace
	// window can be enforced during provisioning. Entries are cleared once the
	// gateway settles (Running, Degraded, or Deployment not Ready). In-memory
	// only: on restart the window restarts, which is acceptable.
	//
	// deploymentNotReadySince records, per gateway, when its Deployment was first
	// observed not Ready (with no rollout in progress) while the gateway was still
	// Provisioning, so the deployment-readiness grace window can be enforced.
	// Cleared once the Deployment is Ready or the gateway leaves Provisioning.
	// In-memory only: on restart the window restarts, which is acceptable.
	mu                      sync.Mutex
	routeNotReadySince      map[string]time.Time
	deploymentNotReadySince map[string]time.Time
	// mu also protects healthAccessCheckedAt. Entries expire after five minutes.
	healthAccessCheckedAt map[string]time.Time

	// vaultSweepProbe memoizes the per-cluster Vault reachability probe for the
	// duration of one health sweep. Every vault-backed gateway shares the single
	// convention Vault address (Vault is a black box, no per-gateway overrides), so a
	// fleet would otherwise probe the same Vault once per gateway each sweep. Reset at
	// the start of reconcileOnce; guarded by mu.
	vaultSweepProbe *vaultProbeResult
}

// vaultProbeResult carries a once-run Vault reachability result shared across the
// gateways of a single health sweep.
type vaultProbeResult struct {
	once sync.Once
	err  error
}

func NewGatewayHealthReconciler(clientset *kubernetes.Clientset, dynamicClient dynamic.Interface, grpcConn *grpc.ClientConn, exposurePort exposure.Port, keycloakConfig *gateway.KeycloakConfig, clusterID, controlPlaneNamespace string) *GatewayHealthReconciler {
	isOpenShift := gateway.DetectOpenShift(clientset)
	return &GatewayHealthReconciler{
		clientset:               clientset,
		dynamicClient:           dynamicClient,
		grpcConn:                grpcConn,
		clusterID:               clusterID,
		versionObserver:         newHTTPGatewayVersionObserver(),
		controlPlaneNamespace:   controlPlaneNamespace,
		interval:                defaultHealthInterval,
		exposure:                exposurePort,
		routeReadyTimeout:       routeReadyTimeout(),
		deploymentReadyTimeout:  deploymentReadyTimeout(),
		keycloakConfig:          keycloakConfig,
		isOpenShift:             isOpenShift,
		now:                     time.Now,
		routeNotReadySince:      make(map[string]time.Time),
		deploymentNotReadySince: make(map[string]time.Time),
	}
}

// concreteClientset returns the production *kubernetes.Clientset backing this
// reconciler. The console helpers in the gateway package are typed against the
// concrete client, whereas h.clientset is the interface type so health passes can
// be driven by a fake in tests. NewGatewayHealthReconciler always wires a
// concrete client, so this returns non-nil in production; the paths that consume
// it are not reached under the interface-only test fakes. Returning nil (rather than panicking on a
// failed assertion) keeps a misconfiguration from crashing the health loop.
func (h *GatewayHealthReconciler) concreteClientset() *kubernetes.Clientset {
	cs, ok := h.clientset.(*kubernetes.Clientset)
	if !ok {
		// Never happens in production (the constructor always wires a concrete
		// client). Log rather than return a silent nil so that if a future test or
		// refactor drives a routed teardown/console path with a non-concrete fake,
		// the misconfiguration is diagnosable here instead of surfacing as an opaque
		// nil dereference deep inside the gateway helpers.
		slog.Warn("Gateway health: clientset is not *kubernetes.Clientset; routed teardown/console helpers require the concrete client", "type", fmt.Sprintf("%T", h.clientset))
	}
	return cs
}

// routeReadyTimeout resolves the route-readiness grace window from
// GATEWAY_ROUTE_READY_TIMEOUT, falling back to the default.
func routeReadyTimeout() time.Duration {
	if v := os.Getenv("GATEWAY_ROUTE_READY_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
		slog.Warn("Invalid GATEWAY_ROUTE_READY_TIMEOUT, using default", "value", v, "default", defaultRouteReadyTimeout)
	}
	return defaultRouteReadyTimeout
}

// deploymentReadyTimeout resolves the deployment-readiness grace window from
// GATEWAY_DEPLOYMENT_READY_TIMEOUT, falling back to the default.
func deploymentReadyTimeout() time.Duration {
	if v := os.Getenv("GATEWAY_DEPLOYMENT_READY_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
		slog.Warn("Invalid GATEWAY_DEPLOYMENT_READY_TIMEOUT, using default", "value", v, "default", defaultDeploymentReadyTimeout)
	}
	return defaultDeploymentReadyTimeout
}

// Run drives the health reconciliation loop until the context is cancelled.
func (h *GatewayHealthReconciler) Run(ctx context.Context) error {
	slog.InfoContext(ctx, "Gateway health reconciler started",
		"interval", h.interval,
		"route_ready_timeout", h.routeReadyTimeout)
	if err := ctx.Err(); err != nil {
		return err
	}
	h.reconcileOnce(ctx)

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
	ctx, endSpan := cpotel.StartReconcileSpan(ctx, "Gateway", "reconcile", "")
	var tickErr error
	defer func() {
		outcome, reason := cpotel.ClassifyReconcileOutcome(tickErr)
		endSpan(outcome, reason, tickErr)
	}()

	client := pb.NewGatewayServiceClient(h.grpcConn)
	// Page through the whole fleet: the list endpoint is server-side paginated
	// (default page size 20), so an unpaged request would only ever refresh the
	// health of the first page of gateways.
	gateways, err := listAllGateways(ctx, client, h.clusterID)
	if err != nil {
		tickErr = err
		slog.WarnContext(ctx, "Gateway health: list gateways failed", "error", err)
		return
	}

	h.pruneHealthAccessChecks()

	// Reset the per-sweep Vault reachability memo so this sweep probes the shared
	// convention Vault at most once across all its vault-backed gateways.
	h.mu.Lock()
	h.vaultSweepProbe = &vaultProbeResult{}
	h.mu.Unlock()

	h.reconcileGateways(ctx, client, gateways)
}

// vaultReachableCached runs the per-cluster Vault reachability probe at most once per
// health sweep, caching the result so a fleet of gateways sharing one convention Vault
// address does not re-probe per gateway. A nil per-sweep memo (a direct unit-test call to
// reconcileGatewayHealth outside reconcileOnce) probes directly without caching.
func (h *GatewayHealthReconciler) vaultReachableCached(ctx context.Context) error {
	probe := h.vaultReachable
	if probe == nil {
		probe = gateway.ProbeVaultReachable
	}
	h.mu.Lock()
	p := h.vaultSweepProbe
	h.mu.Unlock()
	if p == nil {
		return probe(ctx, h.clientset)
	}
	p.once.Do(func() { p.err = probe(ctx, h.clientset) })
	return p.err
}

// reconcileGateways processes different gateways concurrently with a bounded
// worker count. It keeps all work for one gateway in one serial pass. Health
// and console work run before the version request, so a slow version endpoint
// cannot delay a health update for the same gateway.
func (h *GatewayHealthReconciler) reconcileGateways(ctx context.Context, client pb.GatewayServiceClient, gateways []*pb.Gateway) {
	runGatewayWorkers(ctx, gateways, func(gatewayRecord *pb.Gateway) {
		namespace, ready := h.reconcileGatewayHealth(ctx, client, gatewayRecord)
		if ready {
			h.reconcileGatewayVersion(ctx, client, gatewayRecord, namespace)
		}
	})
}

func runGatewayWorkers(ctx context.Context, gateways []*pb.Gateway, reconcile func(*pb.Gateway)) {
	if len(gateways) == 0 {
		return
	}

	workerCount := gatewayHealthWorkerCount
	if len(gateways) < workerCount {
		workerCount = len(gateways)
	}

	jobs := make(chan *pb.Gateway)
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for gatewayRecord := range jobs {
				reconcile(gatewayRecord)
			}
		}()
	}

	seen := make(map[string]struct{}, len(gateways))
sendLoop:
	for _, gatewayRecord := range gateways {
		gatewayID := gatewayRecord.GetMetadata().GetId()
		if gatewayID == "" {
			continue
		}
		if _, exists := seen[gatewayID]; exists {
			continue
		}
		seen[gatewayID] = struct{}{}
		select {
		case jobs <- gatewayRecord:
		case <-ctx.Done():
			break sendLoop
		}
	}
	close(jobs)
	workers.Wait()
}

func (h *GatewayHealthReconciler) reconcileGatewayHealth(ctx context.Context, client pb.GatewayServiceClient, gw *pb.Gateway) (string, bool) {
	gatewayID := gw.GetMetadata().GetId()
	if gatewayID == "" {
		return "", false
	}
	phase := gw.GetPhase()

	// Only gateways the provisioning path has already acted upon carry an
	// observable workload. Leave Pending gateways to the provisioning path and
	// Failed gateways to a subsequent spec change.
	switch phase {
	case string(gatewayhealth.PhaseRunning), string(gatewayhealth.PhaseDegraded), string(gatewayhealth.PhaseProvisioning):
	default:
		return "", false
	}

	// Keep the console_address in sync with the console pod's readiness so the web
	// UI's console button only appears once the console can serve (and disappears
	// if it later goes unready). Independent of the gateway workload's own phase.
	consoleServable := syncConsoleAddress(ctx, h.clientset, h.dynamicClient, client, gatewayID, gw)

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

	namespace, err := gatewayNamespace(gw)
	if err != nil {
		slog.WarnContext(ctx, "Gateway health: reconcile failed", "gateway_id", gatewayID, "error", err)
		return "", false
	}
	ready, rollingOut, reason, err := gateway.ObserveGatewayRollout(ctx, h.clientset, namespace, gateway.GatewayDeploymentName)
	if err != nil {
		slog.WarnContext(ctx, "Gateway health: reconcile failed", "gateway_id", gatewayID, "error", err)
		return "", false
	}

	var desiredPhase, desiredStatus string
	switch {
	case !ready && reason == "deployment not found":
		// The Deployment has not been created yet; the provisioning path still
		// owns this gateway. Leave its phase untouched.
		return namespace, ready
	case !ready && rollingOut:
		// A new revision is rolling out. The provisioning path owns this
		// transition: it holds the gateway at Provisioning and drives it to
		// Running or, when the readiness window elapses, Degraded. Leave the phase
		// untouched so the health loop does not flap it, and do not report the
		// gateway ready. The last-good workload keeps serving throughout
		// (maxUnavailable:0), so a roll in progress is not a degradation. See
		// openshell-gateway-health.spec.md.
		return namespace, false
	case !ready:
		// The updated (current) revision's pods are unavailable and no roll is in
		// progress. For a gateway that has not yet reached Running this is a
		// first-time provisioning whose pods are still pending or starting, held
		// at Provisioning within the grace window; otherwise it is a steady-state
		// degradation of the running release.
		h.clearRouteTimer(gatewayID)
		desiredPhase, desiredStatus = h.evaluateDeploymentReadiness(gatewayID, phase, reason)
	default:
		// Deployment is Ready; the gateway additionally requires its external
		// exposure to be observed Ready before it can be Running.
		h.clearDeploymentTimer(gatewayID)
		desiredPhase, desiredStatus = h.evaluateRouteReadiness(ctx, gatewayID, namespace, gw.GetRouteAddress(), phase)
		if desiredPhase == "" {
			// Transient error observing the exposure; leave the phase untouched
			// rather than flap the gateway.
			return namespace, ready
		}
	}

	// Vault-backed gateways additionally require their per-cluster Vault to be reachable
	// to be Running. A Vault outage degrades the gateway (recoverable once Vault returns)
	// and increments a dashboard-visible signal so the cause is distinguishable from
	// other degradations. See openshell-gateway-credentials.spec.md (Fail Closed When
	// Vault Is Unavailable -- At runtime).
	if desiredPhase == string(gatewayhealth.PhaseRunning) {
		vaultBacked, driverErr := gateway.IsVaultBacked(gw.GetCredentialDriver())
		switch {
		case driverErr != nil:
			// A malformed stored driver is not assumed to be vault: do not probe (or degrade)
			// a gateway whose credential driver we cannot parse. Log and leave the phase.
			slog.WarnContext(ctx, "Gateway health: cannot determine credential driver, skipping Vault probe", "gateway_id", gatewayID, "error", driverErr)
		case vaultBacked:
			if err := h.vaultReachableCached(ctx); err != nil {
				slog.WarnContext(ctx, "Gateway health: provider Vault unreachable", "gateway_id", gatewayID, "error", err)
				cpotel.RecordGatewayVaultUnreachable(ctx, h.clusterID)
				desiredPhase, desiredStatus = string(gatewayhealth.PhaseDegraded), "Vault credential backend unreachable"
			}
		}
	}

	// active_sandbox_count is maintained independently by the event-driven
	// sandbox-count reconciler (see openshell-gateway-sandbox-count.spec.md); the
	// health reconciler only owns phase and status except while the lightweight
	// Keycloak reconciler has published one of its fixed external-state markers.
	update := observedGatewayHealthUpdate(gatewayID, phase, gw.GetStatus(), desiredPhase, desiredStatus)

	// The health reconciler owns finalizing the user-facing provisioning
	// conditions after convergence. The provisioning body acknowledges
	// observed_generation at manifest apply, so the convergence gate suppresses it
	// before its later pass could set GatewayHealthy to Complete once the workload
	// (and, for a routed gateway, its route) is observed ready. Whoever promotes a
	// gateway to Running therefore also completes its conditions, so a Running
	// gateway never reports an unfinished step. Only write when a condition is not
	// already Complete, so a steady-state healthy tick adds no update traffic.
	if desiredPhase == string(gatewayhealth.PhaseRunning) && desiredStatus == gatewayhealth.StatusHealthy {
		if conds := gw.GetProvisioningConditions(); len(conds) > 0 && !allConditionsComplete(conds) {
			if update == nil {
				update = &pb.UpdateGatewayRequest{Id: gatewayID}
			}
			update.ProvisioningConditions = completedConditions(conds)
		}
	}

	if update == nil {
		return namespace, ready
	}

	response, err := client.UpdateGateway(ctx, update)
	if err != nil {
		slog.WarnContext(ctx, "Gateway health: update phase failed", "gateway_id", gatewayID, "phase", desiredPhase, "error", err)
		return namespace, ready
	}
	if isGatewayProvisionCompletion(phase, desiredPhase) {
		observeGatewayProvisionSuccess(ctx, response.GetGateway())
	}

	slog.InfoContext(ctx, "Gateway health: phase changed",
		"gateway_id", gatewayID,
		"from_phase", phase,
		"phase", desiredPhase,
		"status", desiredStatus)
	return namespace, ready
}

// observedGatewayHealthUpdate builds the narrow update required for an observed
// health transition. A healthy workload does not prove that its external
// Keycloak client exists or that its persisted identity is valid, so
// Running/Healthy observations preserve either fixed Keycloak status. The health reconciler may still promote the
// workload phase to Running, but does so with a phase-only update. Unhealthy
// observations retain normal ownership of phase and status so operational
// failures remain visible.
func observedGatewayHealthUpdate(gatewayID, currentPhase, currentStatus, desiredPhase, desiredStatus string) *pb.UpdateGatewayRequest {
	if gatewayID == "" || desiredPhase == "" {
		return nil
	}
	if isGatewayKeycloakClientStatus(currentStatus) && desiredPhase == string(gatewayhealth.PhaseRunning) && desiredStatus == gatewayhealth.StatusHealthy {
		if currentPhase == desiredPhase {
			return nil
		}
		return &pb.UpdateGatewayRequest{
			Id:    gatewayID,
			Phase: &desiredPhase,
		}
	}
	if currentPhase == desiredPhase && currentStatus == desiredStatus {
		return nil
	}
	return &pb.UpdateGatewayRequest{
		Id:     gatewayID,
		Phase:  &desiredPhase,
		Status: &desiredStatus,
	}
}

// allConditionsComplete reports whether every provisioning condition is
// Complete. An empty list is not complete: there is nothing to assert healthy.
func allConditionsComplete(conds []*pb.ProvisioningCondition) bool {
	if len(conds) == 0 {
		return false
	}
	for _, c := range conds {
		if c.GetConditionStatus() != pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_COMPLETE {
			return false
		}
	}
	return true
}

// completedConditions returns a copy of conds with every condition set to
// Complete and its message cleared. The existing condition set and order are
// preserved so the console stepper keeps the same steps it has been showing.
func completedConditions(conds []*pb.ProvisioningCondition) []*pb.ProvisioningCondition {
	out := make([]*pb.ProvisioningCondition, len(conds))
	for i, c := range conds {
		out[i] = &pb.ProvisioningCondition{
			Type:            c.GetType(),
			ConditionStatus: pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_COMPLETE,
		}
	}
	return out
}

// selfHealConsole re-reconciles the per-gateway console when it is observed not
// servable, so a console that failed to provision or has since drifted is
// recreated without a gateway spec change.
// The reconcile is idempotent and its failures are logged, never propagated:
// they must not perturb the gateway's own health phase.
func (h *GatewayHealthReconciler) selfHealConsole(ctx context.Context, gatewayID string, gw *pb.Gateway) {
	namespace, err := gatewayNamespace(gw)
	if err != nil {
		slog.WarnContext(ctx, "Console self-heal failed", "gateway_id", gatewayID, "error", err)
		return
	}
	clientID, err := gatewayKeycloakClientID(gatewayID, gw)
	if err != nil {
		slog.WarnContext(ctx, "Console self-heal: stored identity cannot be resolved", "gateway_id", gatewayID, "error", err)
		return
	}
	opts := gateway.ReconcileOpts{
		IsOpenShift:     h.isOpenShift,
		Keycloak:        h.keycloakConfig,
		GatewayID:       gatewayID,
		GatewayName:     gw.GetName(),
		GatewayClientID: clientID,
	}
	// The self-heal must key on the same route config the console-address
	// publisher uses (consoleRouteConfig). Passing a bare NamespaceConfig here
	// drops the gateway's dedicated-route opt-in, so the self-heal of a
	// dedicated-certificate gateway would re-parent its console onto the shared
	// ingress -- shared host, shared Gateway, shared-host Keycloak redirect --
	// while console_address keeps advertising the dedicated host, leaving the
	// console button pointing at a URL no listener serves.
	nsConfig := gateway.NamespaceConfig{
		Name:    namespace,
		Gateway: gateway.GatewayConfig{Route: consoleRouteConfig(gw)},
	}
	if err := gateway.ReconcileConsole(ctx, h.dynamicClient, h.concreteClientset(), nsConfig, opts); err != nil {
		slog.WarnContext(ctx, "Console self-heal failed", "namespace", namespace, "error", err)
		return
	}
	slog.InfoContext(ctx, "Console self-heal reconciled", "namespace", namespace)
}

// evaluateRouteReadiness decides the phase for a routed gateway whose Deployment
// is Ready, based on its external-exposure readiness. routeAddress is the
// Gateway resource's already-persisted routeAddress (gw.GetRouteAddress()):
// the exposure port treats an empty value as not-Ready even when the
// underlying Gateway API resources are otherwise healthy, since a gateway is
// not reachable until its own address has been published (HYPERSHELL-504). It
// returns an empty phase to signal "leave untouched" when the exposure cannot
// be observed.
//
// The route-readiness grace window applies only while the gateway is still
// provisioning (never yet Running): within the window it stays Provisioning,
// beyond it becomes Degraded. A gateway that had reached Running and then loses
// readiness is moved to Degraded immediately, and a Degraded gateway stays
// Degraded until its exposure recovers.
func (h *GatewayHealthReconciler) evaluateRouteReadiness(ctx context.Context, gatewayID, namespace, routeAddress, currentPhase string) (string, string) {
	rr, err := h.exposure.ObserveReadiness(ctx, exposure.Request{Namespace: namespace, RouteAddress: routeAddress})
	if err != nil {
		slog.WarnContext(ctx, "Gateway health: observe exposure failed", "gateway_id", gatewayID, "error", err)
		return "", ""
	}
	if rr.Ready {
		h.clearRouteTimer(gatewayID)
		return string(gatewayhealth.PhaseRunning), gatewayhealth.StatusHealthy
	}

	if currentPhase == string(gatewayhealth.PhaseProvisioning) {
		since := h.markRouteNotReady(gatewayID)
		if h.now().Sub(since) >= h.routeReadyTimeout {
			h.clearRouteTimer(gatewayID)
			return string(gatewayhealth.PhaseDegraded), fmt.Sprintf("route not ready after %s: %s", h.routeReadyTimeout, rr.Reason)
		}
		return string(gatewayhealth.PhaseProvisioning), rr.Reason
	}

	// currentPhase is Running (lost readiness) or Degraded (still unhealthy).
	h.clearRouteTimer(gatewayID)
	return string(gatewayhealth.PhaseDegraded), rr.Reason
}

// evaluateDeploymentReadiness decides the phase and status for a gateway whose
// Deployment is not Ready and has no rollout in progress.
//
// The deployment-readiness grace window applies only while the gateway is still
// provisioning (never yet Running): its pods may legitimately be pending,
// pulling images, or starting, so within the window it stays Provisioning and
// beyond it becomes Degraded. A gateway that had reached Running and then loses
// readiness is moved to Degraded immediately, and a Degraded gateway stays
// Degraded until its workload recovers.
func (h *GatewayHealthReconciler) evaluateDeploymentReadiness(gatewayID, currentPhase, reason string) (string, string) {
	if currentPhase == string(gatewayhealth.PhaseProvisioning) {
		since := h.markDeploymentNotReady(gatewayID)
		if h.now().Sub(since) >= h.deploymentReadyTimeout {
			h.clearDeploymentTimer(gatewayID)
			return string(gatewayhealth.PhaseDegraded), fmt.Sprintf("deployment not ready after %s: %s", h.deploymentReadyTimeout, reason)
		}
		return string(gatewayhealth.PhaseProvisioning), reason
	}

	// currentPhase is Running (lost readiness) or Degraded (still unhealthy).
	h.clearDeploymentTimer(gatewayID)
	return string(gatewayhealth.PhaseDegraded), reason
}

// markDeploymentNotReady records the first time a provisioning gateway's
// Deployment was observed not Ready, returning that timestamp (existing or now).
func (h *GatewayHealthReconciler) markDeploymentNotReady(gatewayID string) time.Time {
	h.mu.Lock()
	defer h.mu.Unlock()
	if t, ok := h.deploymentNotReadySince[gatewayID]; ok {
		return t
	}
	t := h.now()
	h.deploymentNotReadySince[gatewayID] = t
	return t
}

// clearDeploymentTimer forgets any recorded deployment-not-ready start time for a
// gateway.
func (h *GatewayHealthReconciler) clearDeploymentTimer(gatewayID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.deploymentNotReadySince, gatewayID)
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
