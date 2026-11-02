package reconciler

import (
	"context"
	"fmt"
	"log"
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

func (r *CIProviderReconciler) handleUpsert(ctx context.Context, id string, provider *pb.CIProvider) error {
	alias := deriveIdpAlias(provider.GetForge(), provider.GetIssuer())

	// TODO(PLAT-DATA-24): create or update the validation-only Keycloak IdP:
	//   - IdP alias: alias
	//   - Issuer: provider.GetIssuer()
	//   - JWKS URL: provider.GetJwksUrl()
	//   - Audience: provider.GetAudience()
	//   - No user login flow (validation-only)
	log.Printf("INFO CIProvider %s: would ensure Keycloak IdP alias=%s issuer=%s (Keycloak integration pending)", id, alias, provider.GetIssuer())

	statusIdpAlias := alias
	_, err := r.ciProviderSvc.UpdateCIProvider(ctx, &pb.UpdateCIProviderRequest{
		Id:             id,
		StatusIdpAlias: &statusIdpAlias,
	})
	if err != nil {
		return fmt.Errorf("updating CI provider %s status: %w", id, err)
	}

	log.Printf("INFO CIProvider %s reconciled: idp_alias=%s", id, alias)
	return nil
}

func (r *CIProviderReconciler) handleDelete(ctx context.Context, id string, provider *pb.CIProvider) error {
	alias := provider.GetStatusIdpAlias()
	if alias == "" {
		alias = deriveIdpAlias(provider.GetForge(), provider.GetIssuer())
	}

	// TODO(PLAT-DATA-24): delete the Keycloak IdP by alias
	log.Printf("INFO CIProvider %s: would delete Keycloak IdP alias=%s (Keycloak integration pending)", id, alias)

	return nil
}
