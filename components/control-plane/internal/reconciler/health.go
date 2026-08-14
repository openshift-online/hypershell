package reconciler

import (
	"context"
	"log"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
	"google.golang.org/grpc"
	"k8s.io/client-go/kubernetes"
)

// defaultHealthInterval is the cadence at which the control plane observes
// gateway workload health and synchronizes the Gateway phase.
const defaultHealthInterval = 30 * time.Second

// GatewayHealthReconciler continuously observes the health of provisioned
// gateway Deployments and keeps each Gateway's `phase` and `status`
// synchronized with actual workload state. It runs independently of the
// provisioning phase gate: a Running gateway whose pod begins crash-looping is
// moved to Degraded, and a Degraded gateway whose workload recovers is moved
// back to Running. See openshell-gateway-health.spec.md.
type GatewayHealthReconciler struct {
	clientset *kubernetes.Clientset
	grpcConn  *grpc.ClientConn
	interval  time.Duration
}

func NewGatewayHealthReconciler(clientset *kubernetes.Clientset, grpcConn *grpc.ClientConn) *GatewayHealthReconciler {
	return &GatewayHealthReconciler{
		clientset: clientset,
		grpcConn:  grpcConn,
		interval:  defaultHealthInterval,
	}
}

// Run drives the health reconciliation loop until the context is cancelled.
func (h *GatewayHealthReconciler) Run(ctx context.Context) error {
	log.Printf("INFO gateway health reconciler started (interval=%s)", h.interval)
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
	resp, err := client.ListGateways(ctx, &pb.ListGatewaysRequest{})
	if err != nil {
		log.Printf("WARN gateway health: list gateways: %v", err)
		return
	}

	for _, gw := range resp.GetItems() {
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

	namespace := gatewayNamespace(gw)
	ready, reason, err := gateway.DeploymentReadiness(ctx, h.clientset, namespace, gateway.GatewayDeploymentName)
	if err != nil {
		log.Printf("WARN gateway health: %s: %v", gatewayID, err)
		return
	}

	var desiredPhase, desiredStatus string
	if ready {
		desiredPhase, desiredStatus = "Running", "Healthy"
	} else {
		// The Deployment has not been created yet; the provisioning path still
		// owns this gateway. Leave its phase untouched.
		if reason == "deployment not found" {
			return
		}
		desiredPhase, desiredStatus = "Degraded", reason
	}

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
