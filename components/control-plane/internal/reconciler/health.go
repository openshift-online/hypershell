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
	"google.golang.org/grpc"
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

// GatewayHealthReconciler continuously observes the health of provisioned
// gateway Deployments and, for routed gateways, the readiness of their external
// exposure, and keeps each Gateway's `phase` and `status` synchronized with
// actual state. It runs independently of the provisioning phase gate: a Running
// gateway whose pod begins crash-looping (or whose route loses readiness) is
// moved to Degraded, and a Degraded gateway whose workload and exposure recover
// is moved back to Running. See openshell-gateway-health.spec.md.
type GatewayHealthReconciler struct {
	clientset         *kubernetes.Clientset
	grpcConn          *grpc.ClientConn
	interval          time.Duration
	exposure          exposure.Port
	routeReadyTimeout time.Duration

	// now is the clock, overridable in tests.
	now func() time.Time

	// routeNotReadySince records, per gateway, when its Deployment first became
	// Ready while its external exposure was not, so the route-readiness grace
	// window can be enforced during provisioning. Entries are cleared once the
	// gateway settles (Running, Degraded, or Deployment not Ready). In-memory
	// only: on restart the window restarts, which is acceptable.
	mu                 sync.Mutex
	routeNotReadySince map[string]time.Time
}

func NewGatewayHealthReconciler(clientset *kubernetes.Clientset, grpcConn *grpc.ClientConn, exposurePort exposure.Port) *GatewayHealthReconciler {
	return &GatewayHealthReconciler{
		clientset:          clientset,
		grpcConn:           grpcConn,
		interval:           defaultHealthInterval,
		exposure:           exposurePort,
		routeReadyTimeout:  routeReadyTimeout(),
		now:                time.Now,
		routeNotReadySince: make(map[string]time.Time),
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
	client := pb.NewGatewayServiceClient(h.grpcConn)
	// Page through the whole fleet: the list endpoint is server-side paginated
	// (default page size 20), so an unpaged request would only ever refresh the
	// health of the first page of gateways.
	gateways, err := listAllGateways(ctx, client)
	if err != nil {
		log.Printf("WARN gateway health: list gateways: %v", err)
		return
	}

	for _, gw := range gateways {
		h.reconcileGatewayHealth(ctx, client, gw)
	}
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
