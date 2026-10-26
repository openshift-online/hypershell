package gateways

import (
	"strings"

	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// Gateway field-level write policy (data-model.spec.md "Requirement: Gateway Field
// Ownership and Write Policy", PLAT-DATA-23; security/rbac-enforcement.spec.md
// "Requirement: Gateway Field Write Policy", SEC-RBAC-18).
//
// The Gateway resource exposes exactly ONE non-privileged-writable field after create
// -- name -- plus placement at create. Every other field is owned by the control plane.
// The policy is a single, authoritative, default-deny allowlist enforced in the gateway
// service write funnel (Create / Replace), so REST, gRPC, and the declarative
// .hypershell import are all covered at one chokepoint. Only a control-plane internal
// reconcile write (GatewayWriterControlPlane) bypasses it; a user and a GitOps import
// are both held to the allowlist.

// gatewayFieldOwnership classifies every Gateway model field by json tag so the policy
// and the field-census test share one source of truth. The census test asserts that
// every reflected Gateway field appears here, so a newly added field without a decision
// FAILS the build (default-deny: PLAT-DATA-23.07).
type gatewayFieldClass int

const (
	// fieldUserWritable is writable by a user or import caller: name (create+update)
	// and the create-only placement inputs (placement, cluster_id).
	fieldUserWritable gatewayFieldClass = iota
	// fieldPlatformManaged is the control-plane-only desired workload spec.
	fieldPlatformManaged
	// fieldObserved is a control-plane-only observed field.
	fieldObserved
	// fieldServerOwned is owned by the API server/DB and written by no caller.
	fieldServerOwned
)

// gatewayFieldOwnership maps each Gateway model json tag to its ownership class. Keep it
// in sync with the Gateway struct in model.go; the field-census test enforces that.
var gatewayFieldOwnership = map[string]gatewayFieldClass{
	// User-writable allowlist.
	"name":       fieldUserWritable,
	"cluster_id": fieldUserWritable, // create-only (placement); control-plane-assigned on late-bind, not user-writable on update
	"placement":  fieldUserWritable, // create-only

	// Platform-managed (control-plane-only desired workload spec).
	"external_dns":      fieldPlatformManaged,
	"tls_mode":          fieldPlatformManaged,
	"service_type":      fieldPlatformManaged,
	"image":             fieldPlatformManaged,
	"supervisor_image":  fieldPlatformManaged,
	"server_dns_names":  fieldPlatformManaged,
	"oidc":              fieldPlatformManaged,
	"route":             fieldPlatformManaged,
	"credential_driver": fieldPlatformManaged,
	"workspace_mode":    fieldPlatformManaged, // server-defaulted to managed at create (OSG-CORE-18); no caller may set or change it

	// Control-plane-observed.
	"status":                  fieldObserved,
	"phase":                   fieldObserved,
	"route_address":           fieldObserved,
	"console_address":         fieldObserved,
	"gateway_version":         fieldObserved,
	"observed_generation":     fieldObserved,
	"active_sandbox_count":    fieldObserved,
	"provisioning_conditions": fieldObserved,

	// Server-owned (no caller writes).
	"namespace":     fieldServerOwned,
	"generation":    fieldServerOwned,
	"repository_id": fieldServerOwned,
}

// createWorkloadFields are the platform-managed workload fields that are never set by
// the API server during create. Any value present at create therefore came from the
// caller, so for a user/import create the presence of one is a policy violation. Observed
// fields (status, phase) are excluded here because the create handler legitimately sets
// phase/status server-side for an awaiting-placement gateway; a caller-supplied observed
// field is rejected earlier, at the request boundary.
func gatewayCreateWorkloadViolation(g *Gateway) string {
	switch {
	case g.ExternalDns != nil:
		return "external_dns"
	case g.TlsMode != nil:
		return "tls_mode"
	case g.ServiceType != nil:
		return "service_type"
	case g.Image != nil:
		return "image"
	case g.SupervisorImage != nil:
		return "supervisor_image"
	case g.ServerDnsNames != nil:
		return "server_dns_names"
	case g.Oidc != nil:
		return "oidc"
	case g.Route != nil:
		return "route"
	case g.CredentialDriver != nil:
		return "credential_driver"
	case g.WorkspaceMode != nil:
		return "workspace_mode"
	}
	return ""
}

// enforceGatewayFieldPolicyCreate rejects a user or import create that carries a
// platform-managed workload field. A control-plane internal write retains full access.
// It is the authoritative service-funnel backstop for create (SEC-RBAC-18); the create
// handlers reject caller-supplied forbidden fields earlier with the same field names.
func enforceGatewayFieldPolicyCreate(kind rbac.GatewayWriterKind, incoming *Gateway) *errors.ServiceError {
	if kind.Privileged() {
		return nil
	}
	if field := gatewayCreateWorkloadViolation(incoming); field != "" {
		return gatewayFieldForbidden(kind, field)
	}
	return nil
}

// enforceGatewayFieldPolicyUpdate rejects a user or import update that changes any
// field outside the update allowlist (name). Re-sending an unchanged value is a no-op,
// so idempotent clients that resend the whole object still work (PLAT-DATA-23.03). A
// control-plane internal write retains full access.
func enforceGatewayFieldPolicyUpdate(kind rbac.GatewayWriterKind, current, incoming *Gateway) *errors.ServiceError {
	if kind.Privileged() {
		return nil
	}
	if field := gatewayUpdateViolation(current, incoming); field != "" {
		return gatewayFieldForbidden(kind, field)
	}
	return nil
}

// gatewayUpdateViolation returns the name of the first non-allowlisted field whose
// incoming value differs from the stored one, or "" when only name (and nothing else)
// changed. name is the sole update-allowlisted field; cluster_id is control-plane
// assigned by late binding and is NOT user-writable on update.
func gatewayUpdateViolation(current, incoming *Gateway) string {
	switch {
	case current.ClusterId != incoming.ClusterId:
		return "cluster_id"
	case !jsonEq(current.Placement, incoming.Placement):
		return "placement"
	case !strEq(current.ExternalDns, incoming.ExternalDns):
		return "external_dns"
	case !strEq(current.TlsMode, incoming.TlsMode):
		return "tls_mode"
	case !strEq(current.ServiceType, incoming.ServiceType):
		return "service_type"
	case !strEq(current.Image, incoming.Image):
		return "image"
	case !strEq(current.SupervisorImage, incoming.SupervisorImage):
		return "supervisor_image"
	case !jsonEq(current.ServerDnsNames, incoming.ServerDnsNames):
		return "server_dns_names"
	case !jsonEq(current.Oidc, incoming.Oidc):
		return "oidc"
	case !jsonEq(current.Route, incoming.Route):
		return "route"
	case !jsonEq(current.CredentialDriver, incoming.CredentialDriver):
		return "credential_driver"
	case !strEq(current.WorkspaceMode, incoming.WorkspaceMode):
		return "workspace_mode"
	case !strEq(current.Status, incoming.Status):
		return "status"
	case !strEq(current.Phase, incoming.Phase):
		return "phase"
	case !strEq(current.RouteAddress, incoming.RouteAddress):
		return "route_address"
	case !strEq(current.ConsoleAddress, incoming.ConsoleAddress):
		return "console_address"
	case !strEq(current.GatewayVersion, incoming.GatewayVersion):
		return "gateway_version"
	case !jsonEq(current.ProvisioningConditions, incoming.ProvisioningConditions):
		return "provisioning_conditions"
	case !intPtrEq(current.ActiveSandboxCount, incoming.ActiveSandboxCount):
		return "active_sandbox_count"
	case current.ObservedGeneration != incoming.ObservedGeneration:
		return "observed_generation"
	}
	return ""
}

// gatewayFieldForbidden builds the field-policy rejection. It is a 403 Forbidden over
// REST and maps to PermissionDenied over gRPC (grpcutil.ServiceErrorToGRPC). The field
// is named so a user, or a git author whose import failed, sees exactly which field is
// platform-managed.
func gatewayFieldForbidden(kind rbac.GatewayWriterKind, field string) *errors.ServiceError {
	return errors.Forbidden(
		"field %q is control-plane-managed and cannot be set by a %s caller", field, kind)
}

// intPtrEq reports whether two optional ints are equal, treating nil as absent.
func intPtrEq(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// defaultGatewayWorkspaceMode is applied by the API server to every gateway
// created without a workspace_mode. Legacy gateways (NULL) keep the chart's
// shared default so the flip applies to new provisioning only
// (openshell-gateway.spec.md, "OSG-CORE-18 -- Gateway Workspace Mode").
const defaultGatewayWorkspaceMode = "managed"

// gatewayWorkspaceModes is the vocabulary shared with the vendored OpenShell
// chart's workspace_mode validation (shared, managed, operator).
var gatewayWorkspaceModes = []string{"shared", "managed", "operator"}

// validateGatewayWorkspaceValue checks a workspace_mode value against the
// accepted vocabulary.
func validateGatewayWorkspaceValue(mode string) *errors.ServiceError {
	for _, allowed := range gatewayWorkspaceModes {
		if mode == allowed {
			return nil
		}
	}
	return errors.Validation("workspace_mode %q is not valid; allowed: %v", mode, gatewayWorkspaceModes)
}

// applyGatewayWorkspaceCreateDefault defaults a created gateway's workspace_mode
// to managed. The field is platform-managed (PLAT-DATA-23): the field policy
// above has already rejected a user/import-supplied value, so a non-nil value
// reaching here can only come from a control-plane internal write -- its
// vocabulary is validated regardless, as defense in depth.
func applyGatewayWorkspaceCreateDefault(gateway *Gateway) *errors.ServiceError {
	if gateway.WorkspaceMode == nil || strings.TrimSpace(*gateway.WorkspaceMode) == "" {
		mode := defaultGatewayWorkspaceMode
		gateway.WorkspaceMode = &mode
		return nil
	}
	trimmed := strings.TrimSpace(*gateway.WorkspaceMode)
	if svcErr := validateGatewayWorkspaceValue(trimmed); svcErr != nil {
		return svcErr
	}
	gateway.WorkspaceMode = &trimmed
	return nil
}

// enforceGatewayWorkspaceUpdateSemantics enforces the value contract on update
// for every writer class, the control plane included: the vocabulary is
// enforced, a stored non-empty value is immutable (the same contract as
// credential_driver), and a NULL value on a legacy gateway may be set exactly
// once -- which, under the field policy, only a control-plane internal write
// can exercise. A next value of nil (or an explicit clear) leaves the stored
// value untouched.
func enforceGatewayWorkspaceUpdateSemantics(current, next *Gateway) *errors.ServiceError {
	if next.WorkspaceMode == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*next.WorkspaceMode)
	if trimmed == "" {
		next.WorkspaceMode = current.WorkspaceMode
		return nil
	}
	if svcErr := validateGatewayWorkspaceValue(trimmed); svcErr != nil {
		return svcErr
	}
	if current.WorkspaceMode != nil && *current.WorkspaceMode != "" && *current.WorkspaceMode != trimmed {
		return errors.Conflict("workspace mode cannot be changed after the gateway is provisioned; recreate the gateway with the desired mode")
	}
	next.WorkspaceMode = &trimmed
	return nil
}
