package reconciler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/keycloak"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
	"google.golang.org/grpc"
)

// defaultCIFederationClientID is the shared, public (keyless) token-exchange
// requester the gitops realm configuration provisions (hypershell-gitops#1636).
// The CI run's OIDC token is the only credential; the client holds no secret.
const defaultCIFederationClientID = "ci-federation"

// ciFederationKeycloak is the Keycloak surface the CITrust reconciler needs. It
// is an interface so the reconciler can be unit-tested against a fake; the
// production implementation is *keycloak.Client.
type ciFederationKeycloak interface {
	EnsureCIFederation(ctx context.Context, spec keycloak.CIFederationSpec) error
	DeleteCIFederation(ctx context.Context, spec keycloak.CIFederationSpec) error
	Issuer() string
}

// CITrustReconciler reconciles CITrust resources into Keycloak token-exchange
// artifacts: a per-trust scoped claim-to-workspace IdP mapper (the subject trust
// boundary), a token-exchange authorization onto the gateway client, and a gateway
// RBAC binding on the derived workspace. The token-exchange authorization mechanism
// is config-driven (PLAT-CITR-08): tokenExchangeMode selects fgap-v1 fine-grained
// grants (PLAT-CITR-11) or the external-to-internal v2 client attribute. Each
// CITrust declares that CI workloads matching a subject predicate may federate into
// a specific gateway with a given role, keyless.
//
// On upsert: resolve the CIProvider (forge + adopted IdP alias) and the gateway
// (Keycloak client + owning cluster), derive the forge-namespaced workspace,
// realize the Keycloak artifacts, then write back status (exchange_endpoint,
// workspace, workflow_hint). On delete: revoke the trust. The reconcile is
// cluster-scoped: only the control plane whose cluster hosts the gateway acts,
// mirroring the RoleBinding reconciler.
type CITrustReconciler struct {
	keycloakClient     ciFederationKeycloak
	ciTrustSvc         pb.CITrustServiceClient
	ciProviderSvc      pb.CIProviderServiceClient
	gatewaySvc         pb.GatewayServiceClient
	federationClientID string
	tokenExchangeMode  keycloak.TokenExchangeMode
	clusterID          string
}

func NewCITrustReconciler(kc *keycloak.Client, conn *grpc.ClientConn, clusterID string, tokenExchangeMode keycloak.TokenExchangeMode) *CITrustReconciler {
	var client ciFederationKeycloak
	// A typed-nil *keycloak.Client stored in an interface is non-nil, which would
	// defeat the nil guard in Handle; only wrap a real client.
	if kc != nil {
		client = kc
	}
	return &CITrustReconciler{
		keycloakClient:     client,
		ciTrustSvc:         pb.NewCITrustServiceClient(conn),
		ciProviderSvc:      pb.NewCIProviderServiceClient(conn),
		gatewaySvc:         pb.NewGatewayServiceClient(conn),
		federationClientID: defaultCIFederationClientID,
		tokenExchangeMode:  tokenExchangeMode,
		clusterID:          clusterID,
	}
}

// deriveWorkspace returns the forge-namespaced workspace name from the CI trust's
// subject. GitHub repositories use "gh-<owner>"; GitLab projects use
// "gl-<top-level-namespace>". This isolation prevents cross-forge collisions and
// scopes the token-exchange to a single workspace per repository owner or project
// namespace.
func deriveWorkspace(forge, subjectRepository, subjectProjectPath string) string {
	switch forge {
	case "github":
		return "gh-" + strings.ToLower(keycloak.OwnerFromRepository(subjectRepository))
	case "gitlab":
		ns := subjectProjectPath
		if idx := strings.Index(subjectProjectPath, "/"); idx >= 0 {
			ns = subjectProjectPath[:idx]
		}
		return "gl-" + strings.ToLower(ns)
	case "forgejo":
		return "fj-" + strings.ToLower(keycloak.OwnerFromRepository(subjectRepository))
	default:
		return "ci-" + strings.ToLower(forge)
	}
}

func (r *CITrustReconciler) Handle(ctx context.Context, event watcher.Event[*pb.CITrust]) error {
	trust := event.Resource
	if trust == nil {
		return nil
	}
	id := event.ResourceID

	switch event.Type {
	case watcher.EventDeleted:
		return r.handleDelete(ctx, id, trust)
	default:
		return r.handleUpsert(ctx, id, trust)
	}
}

func (r *CITrustReconciler) handleUpsert(ctx context.Context, id string, trust *pb.CITrust) error {
	provider, err := r.resolveProvider(ctx, trust)
	if err != nil {
		return err
	}

	gatewayClientID, gatewayCluster, err := r.resolveGatewayClientID(ctx, trust.GetGatewayId())
	if err != nil {
		if isNotFoundRPC(err) {
			// The gateway is not yet visible (provisioning race) or already gone.
			// The exchange cannot be authorized onto a client that does not exist;
			// return the error so it is logged and the trust is re-reconciled when
			// the watch next reseeds current state (reconnect / idle timeout).
			return fmt.Errorf("CITrust %s: gateway %s not found: %w", id, trust.GetGatewayId(), err)
		}
		return fmt.Errorf("CITrust %s: resolve gateway %s: %w", id, trust.GetGatewayId(), err)
	}
	if gatewayCluster != r.clusterID {
		slog.DebugContext(ctx, "CITrust gateway belongs to another cluster, skipping",
			"ci_trust_id", id,
			"gateway_id", trust.GetGatewayId(),
			"gateway_cluster", gatewayCluster,
			"cluster", r.clusterID)
		return nil
	}

	spec := r.federationSpec(id, provider, trust, gatewayClientID)

	if err := r.keycloakClient.EnsureCIFederation(ctx, spec); err != nil {
		var notFound *keycloak.ClientNotFoundError
		if errors.As(err, &notFound) {
			// The gateway's Keycloak client has not been provisioned yet. Return the
			// error so it is logged and retried on the next watch reseed, rather than
			// failing terminally.
			return fmt.Errorf("CITrust %s: gateway keycloak client not ready: %w", id, err)
		}
		return fmt.Errorf("CITrust %s: reconcile keycloak federation: %w", id, err)
	}

	exchangeEndpoint := r.keycloakClient.Issuer() + "/protocol/openid-connect/token"
	workflowHint := workflowHint(provider.GetForge(), exchangeEndpoint, provider.GetAudience(), r.federationClientID, spec.IdpAlias, gatewayClientID, spec.Workspace)
	// Write status only when it changed. Every write emits a watch event that
	// reconciles this trust again (STD-RCON-02.01). The federation step above
	// still runs on every pass, so Keycloak drift is still repaired.
	if trust.GetStatusExchangeEndpoint() == exchangeEndpoint &&
		trust.GetStatusWorkspace() == spec.Workspace &&
		trust.GetStatusWorkflowHint() == workflowHint {
		slog.DebugContext(ctx, "CITrust status already current", "ci_trust_id", id)
		return nil
	}
	if _, err := r.ciTrustSvc.UpdateCITrust(ctx, &pb.UpdateCITrustRequest{
		Id:                     id,
		StatusExchangeEndpoint: &exchangeEndpoint,
		StatusWorkspace:        &spec.Workspace,
		StatusWorkflowHint:     &workflowHint,
	}); err != nil {
		return fmt.Errorf("CITrust %s: update status: %w", id, err)
	}

	slog.InfoContext(ctx, "CITrust reconciled",
		"ci_trust_id", id,
		"workspace", spec.Workspace,
		"gateway_client", gatewayClientID,
		"idp", spec.IdpAlias,
		"role", trust.GetRole())
	return nil
}

func (r *CITrustReconciler) handleDelete(ctx context.Context, id string, trust *pb.CITrust) error {
	provider, err := r.resolveProvider(ctx, trust)
	if err != nil {
		if isNotFoundRPC(err) {
			// Referential integrity (PLAT-CITR-05.01) keeps a CIProvider alive while
			// a CITrust references it, so a missing provider on delete means the
			// provider (and its adopted IdP context) is already torn down. The IdP
			// alias that targets the per-trust mapper is then unknowable, so there is
			// nothing left for this reconciler to revoke.
			slog.WarnContext(ctx, "CITrust CI provider not found on delete; cannot resolve IdP alias to revoke", "ci_trust_id", id, "ci_provider_id", trust.GetCiProviderId())
			return nil
		}
		return err
	}

	// Determine ownership where possible. If the gateway is already gone (cascade
	// delete), the per-trust IdP mapper cleanup is still safe and idempotent, so
	// proceed; if the gateway resolves to another cluster, this control plane does
	// not own it and must not touch the shared realm on its behalf.
	gatewayClientID := ""
	if clientID, gatewayCluster, resolveErr := r.resolveGatewayClientID(ctx, trust.GetGatewayId()); resolveErr != nil {
		if !isNotFoundRPC(resolveErr) {
			return fmt.Errorf("CITrust %s: resolve gateway %s: %w", id, trust.GetGatewayId(), resolveErr)
		}
		slog.DebugContext(ctx, "CITrust gateway no longer exists; revoking shared IdP mapper", "ci_trust_id", id, "gateway_id", trust.GetGatewayId())
	} else {
		if gatewayCluster != r.clusterID {
			slog.DebugContext(ctx, "CITrust gateway belongs to another cluster, skipping",
				"ci_trust_id", id,
				"gateway_id", trust.GetGatewayId(),
				"gateway_cluster", gatewayCluster,
				"cluster", r.clusterID)
			return nil
		}
		gatewayClientID = clientID
	}

	spec := r.federationSpec(id, provider, trust, gatewayClientID)
	if err := r.keycloakClient.DeleteCIFederation(ctx, spec); err != nil {
		return fmt.Errorf("CITrust %s: revoke keycloak federation: %w", id, err)
	}

	slog.InfoContext(ctx, "CITrust revoked", "ci_trust_id", id, "workspace", spec.Workspace, "idp", spec.IdpAlias)
	return nil
}

// resolveProvider fetches the CITrust's CIProvider and returns it. The IdP alias
// the exchange uses is the provider's realized status.idp_alias (the alias
// Keycloak actually holds, per PLAT-CITR-01 adoption); see federationSpec.
func (r *CITrustReconciler) resolveProvider(ctx context.Context, trust *pb.CITrust) (*pb.CIProvider, error) {
	resp, err := r.ciProviderSvc.GetCIProvider(ctx, &pb.GetCIProviderRequest{Id: trust.GetCiProviderId()})
	if err != nil {
		return nil, fmt.Errorf("fetch CI provider %s: %w", trust.GetCiProviderId(), err)
	}
	provider := resp.GetCiProvider()
	if provider == nil {
		return nil, fmt.Errorf("CI provider %s not found", trust.GetCiProviderId())
	}
	return provider, nil
}

// federationSpec compiles the Keycloak desired state for a CITrust. The IdP alias
// is the provider's realized status.idp_alias; the control plane adopts the
// platform-provisioned IdP and never creates a parallel one (PLAT-CITR-01), so a
// provider whose alias is not yet realized yields an empty alias and
// EnsureCIFederation fails closed.
func (r *CITrustReconciler) federationSpec(id string, provider *pb.CIProvider, trust *pb.CITrust, gatewayClientID string) keycloak.CIFederationSpec {
	workspace := deriveWorkspace(provider.GetForge(), trust.GetSubjectRepository(), trust.GetSubjectProjectPath())
	return keycloak.CIFederationSpec{
		TrustID:            id,
		Forge:              provider.GetForge(),
		IdpAlias:           provider.GetStatusIdpAlias(),
		FederationClientID: r.federationClientID,
		GatewayClientID:    gatewayClientID,
		Workspace:          workspace,
		Role:               trust.GetRole(),
		Mode:               r.tokenExchangeMode,
		Subject: keycloak.CISubjectPredicate{
			Repository:      trust.GetSubjectRepository(),
			RepositoryOwner: keycloak.OwnerFromRepository(trust.GetSubjectRepository()),
			ProjectPath:     trust.GetSubjectProjectPath(),
			Environment:     trust.GetSubjectEnvironment(),
			Ref:             trust.GetSubjectRef(),
			RefProtected:    trust.GetSubjectRefProtected(),
		},
	}
}

// resolveGatewayClientID looks up the gateway by id and returns its stored
// Keycloak client id (see gatewayKeycloakClientID), plus the cluster the gateway is assigned to so the caller can tell whether this
// control plane owns it. It mirrors the RoleBinding reconciler's resolution.
func (r *CITrustReconciler) resolveGatewayClientID(ctx context.Context, gatewayID string) (string, string, error) {
	resp, err := r.gatewaySvc.GetGateway(ctx, &pb.GetGatewayRequest{Id: gatewayID})
	if err != nil {
		return "", "", fmt.Errorf("get gateway %s: %w", gatewayID, err)
	}
	gw := resp.GetGateway()
	clientID, err := gatewayKeycloakClientID(gatewayID, gw)
	if err != nil {
		return "", "", fmt.Errorf("resolve Keycloak client for gateway %s: %w", gatewayID, err)
	}
	return clientID, gw.GetClusterId(), nil
}

// workflowHint renders a runnable, secret-free CI snippet hint for the provider's
// forge that mints the OIDC token with the pinned audience and exchanges it at
// the cluster Keycloak (PLAT-CITR-06).
func workflowHint(forge, exchangeEndpoint, audience, federationClientID, idpAlias, gatewayClientID, workspace string) string {
	return fmt.Sprintf(
		"forge=%s workspace=%s: mint an OIDC token with aud=%s, then POST grant_type=urn:ietf:params:oauth:grant-type:token-exchange "+
			"client_id=%s subject_issuer=%s subject_token_type=urn:ietf:params:oauth:token-type:jwt audience=%s to %s",
		forge, workspace, audience, federationClientID, idpAlias, gatewayClientID, exchangeEndpoint,
	)
}
