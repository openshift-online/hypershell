package reconciler

import (
	"strings"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
)

// DeployedBundle returns the value the controller writes to each gateway's
// deployed_bundle (data-model.spec.md PLAT-DATA-34): the configured release
// bundle (HYPERSHELL_BUNDLE) when set, otherwise the controller's own image
// reference as read from its Pod status by OwnImage (digest-pinned when the
// kubelet reports a digest, PLAT-PGP-09). Empty when neither is known.
func DeployedBundle(releaseBundle, ownImage string) string {
	if b := strings.TrimSpace(releaseBundle); b != "" {
		return b
	}
	return strings.TrimSpace(ownImage)
}

// SetDeployedBundle sets the identity written to deployed_bundle after every
// successful provisioning pass. Empty leaves deployed_bundle unchanged.
func (r *GatewayReconciler) SetDeployedBundle(bundle string) {
	r.deployedBundle = bundle
}

// convergedUpdateRequest is the single write a successful provisioning pass
// makes: it acknowledges the applied generation (PLAT-DATA-06.03) and records
// the build that applied it (PLAT-DATA-34). A failed pass never builds it, so
// deployed_bundle keeps naming the last build that succeeded.
func convergedUpdateRequest(gatewayID string, generation int64, bundle string) *pb.UpdateGatewayRequest {
	req := &pb.UpdateGatewayRequest{
		Id:                 gatewayID,
		ObservedGeneration: &generation,
	}
	if bundle != "" {
		req.DeployedBundle = &bundle
	}
	return req
}
