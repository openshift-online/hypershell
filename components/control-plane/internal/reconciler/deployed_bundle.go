package reconciler

import (
	"fmt"
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

// currentTargetBundle is the reserved target_bundle that means "the bundle
// this gateway's controller runs" (PLAT-DATA-33.06), for clusters without a
// bundle name such as Kind and ephemeral PR environments.
const currentTargetBundle = "current"

// targetBundleBase drops the "#<token>" reapply suffix (PLAT-DATA-33): the
// token only forces a new generation and is never compared.
func targetBundleBase(target string) string {
	base, _, _ := strings.Cut(target, "#")
	return strings.TrimSpace(base)
}

// targetBundleBlocked reports whether this controller must not apply the
// gateway because its target_bundle names another bundle (PLAT-DATA-33.03),
// and the user-facing reason. No target and "current" never block.
func targetBundleBlocked(target, own string) (string, bool) {
	base := targetBundleBase(target)
	if base == "" || base == currentTargetBundle || base == own {
		return "", false
	}
	if own == "" {
		return fmt.Sprintf("target bundle %s is not available on this cluster; its controller has no bundle identity", base), true
	}
	return fmt.Sprintf("target bundle %s is not available on this cluster; its controller runs %s", base, own), true
}

// conditionMatches reports whether gw already shows step with status and
// message, so a repeated pass does not rewrite (and re-trigger) it.
func conditionMatches(gw *pb.Gateway, step string, status pb.ProvisioningConditionStatus, message string) bool {
	for _, c := range gw.GetProvisioningConditions() {
		if c.GetType() == step {
			return c.GetConditionStatus() == status && c.GetMessage() == message
		}
	}
	return false
}
