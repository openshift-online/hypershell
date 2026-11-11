package reconciler

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/keycloak"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
	"google.golang.org/grpc"
)

// CIProviderReconciler reconciles CIProvider resources into Keycloak as
// validation-only identity providers. Each CIProvider maps to one Keycloak IdP
// whose only purpose is to validate CI OIDC tokens from the registered issuer;
// no user login flow is attached.
//
// On upsert: ensure the Keycloak IdP exists with the provider's issuer, JWKS
// URL, and audience, then write back status.idp_alias. On delete: remove the
// Keycloak IdP.
type CIProviderReconciler struct {
	keycloakClient *keycloak.Client
	ciProviderSvc  pb.CIProviderServiceClient
}

func NewCIProviderReconciler(kc *keycloak.Client, conn *grpc.ClientConn) *CIProviderReconciler {
	return &CIProviderReconciler{
		keycloakClient: kc,
		ciProviderSvc:  pb.NewCIProviderServiceClient(conn),
	}
}

// deriveIdpAlias produces a stable, human-readable Keycloak IdP alias from the
// provider's forge and issuer URL. The alias is deterministic so repeated
// reconciles converge to the same IdP rather than creating duplicates.
func deriveIdpAlias(forge, issuer string) string {
	host := issuer
	if idx := strings.Index(issuer, "://"); idx >= 0 {
		host = issuer[idx+3:]
	}
	host = strings.TrimRight(host, "/")
	host = strings.ReplaceAll(host, "/", "-")
	host = strings.ReplaceAll(host, ".", "-")
	return fmt.Sprintf("ci-%s-%s", forge, host)
}

func (r *CIProviderReconciler) Handle(ctx context.Context, event watcher.Event[*pb.CIProvider]) error {
	provider := event.Resource
	if provider == nil {
		return nil
	}
	id := event.ResourceID

	switch event.Type {
	case watcher.EventDeleted:
		return r.handleDelete(ctx, id, provider)
	default:
		return r.handleUpsert(ctx, id, provider)
	}
}

// resolveIdpAlias selects the Keycloak IdP alias the control plane adopts for a
// provider: the explicit spec idp_alias (PLAT-DATA-24, PLAT-CITR-01) when set - the
// single declarative source of truth shared with the terraform federation so the
// control plane adopts exactly the IdP terraform provisioned - and otherwise the
// alias derived from forge and issuer.
func resolveIdpAlias(provider *pb.CIProvider) string {
	if alias := provider.GetIdpAlias(); alias != "" {
		return alias
	}
	return deriveIdpAlias(provider.GetForge(), provider.GetIssuer())
}

func (r *CIProviderReconciler) handleUpsert(ctx context.Context, id string, provider *pb.CIProvider) error {
	alias := resolveIdpAlias(provider)

	// TODO(PLAT-DATA-24): create or update the validation-only Keycloak IdP:
	//   - IdP alias: alias
	//   - Issuer: provider.GetIssuer()
	//   - JWKS URL: provider.GetJwksUrl()
	//   - Audience: provider.GetAudience()
	//   - No user login flow (validation-only)
	slog.InfoContext(ctx, "CIProvider would ensure Keycloak IdP (Keycloak integration pending)",
		"ci_provider_id", id,
		"idp_alias", alias,
		"issuer", provider.GetIssuer())

	statusIdpAlias := alias
	_, err := r.ciProviderSvc.UpdateCIProvider(ctx, &pb.UpdateCIProviderRequest{
		Id:             id,
		StatusIdpAlias: &statusIdpAlias,
	})
	if err != nil {
		return fmt.Errorf("updating CI provider %s status: %w", id, err)
	}

	slog.InfoContext(ctx, "CIProvider reconciled", "ci_provider_id", id, "idp_alias", alias)
	return nil
}

func (r *CIProviderReconciler) handleDelete(ctx context.Context, id string, provider *pb.CIProvider) error {
	alias := provider.GetStatusIdpAlias()
	if alias == "" {
		alias = resolveIdpAlias(provider)
	}

	// TODO(PLAT-DATA-24): delete the Keycloak IdP by alias
	slog.InfoContext(ctx, "CIProvider would delete Keycloak IdP (Keycloak integration pending)", "ci_provider_id", id, "idp_alias", alias)

	return nil
}
