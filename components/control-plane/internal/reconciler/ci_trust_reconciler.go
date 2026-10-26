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

// CITrustReconciler reconciles CITrust resources into Keycloak claim mappers,
// token-exchange permissions, and gateway RBAC bindings. Each CITrust declares
// that CI workloads matching a subject predicate may federate into a specific
// gateway with a given role.
//
// On upsert: derive the workspace name from the subject (forge-namespaced
// isolation), ensure the Keycloak claim mapper and token-exchange permission,
// bind the gateway RBAC role, then write back status (exchange_endpoint,
// workspace, workflow_hint). On delete: remove the Keycloak artifacts and RBAC
// binding.
type CITrustReconciler struct {
	keycloakClient *keycloak.Client
	ciTrustSvc     pb.CITrustServiceClient
	ciProviderSvc  pb.CIProviderServiceClient
}

func NewCITrustReconciler(kc *keycloak.Client, conn *grpc.ClientConn) *CITrustReconciler {
	return &CITrustReconciler{
		keycloakClient: kc,
		ciTrustSvc:     pb.NewCITrustServiceClient(conn),
		ciProviderSvc:  pb.NewCIProviderServiceClient(conn),
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
		owner := subjectRepository
		if idx := strings.Index(subjectRepository, "/"); idx >= 0 {
			owner = subjectRepository[:idx]
		}
		return "gh-" + strings.ToLower(owner)
	case "gitlab":
		ns := subjectProjectPath
		if idx := strings.Index(subjectProjectPath, "/"); idx >= 0 {
			ns = subjectProjectPath[:idx]
		}
		return "gl-" + strings.ToLower(ns)
	case "forgejo":
		owner := subjectRepository
		if idx := strings.Index(subjectRepository, "/"); idx >= 0 {
			owner = subjectRepository[:idx]
		}
		return "fj-" + strings.ToLower(owner)
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
	providerResp, err := r.ciProviderSvc.GetCIProvider(ctx, &pb.GetCIProviderRequest{
		Id: trust.GetCiProviderId(),
	})
	if err != nil {
		return fmt.Errorf("fetching CI provider %s for trust %s: %w", trust.GetCiProviderId(), id, err)
	}
	provider := providerResp.GetCiProvider()

	workspace := deriveWorkspace(
		provider.GetForge(),
		trust.GetSubjectRepository(),
		trust.GetSubjectProjectPath(),
	)

	// TODO(PLAT-DATA-25): reconcile Keycloak artifacts:
	//   1. Create/update claim mapper on the provider's IdP for the subject predicate
	//   2. Enable token-exchange permission from the CI client to the gateway client
	//   3. Bind the gateway RBAC role (trust.GetRole()) to the workspace identity
	log.Printf("INFO CITrust %s: would reconcile Keycloak artifacts for gateway=%s workspace=%s role=%s (Keycloak integration pending)",
		id, trust.GetGatewayId(), workspace, trust.GetRole())

	statusWorkspace := workspace
	statusWorkflowHint := fmt.Sprintf("openshell token-exchange --workspace %s --gateway %s", workspace, trust.GetGatewayId())
	_, err = r.ciTrustSvc.UpdateCITrust(ctx, &pb.UpdateCITrustRequest{
		Id:                  id,
		StatusWorkspace:     &statusWorkspace,
		StatusWorkflowHint:  &statusWorkflowHint,
	})
	if err != nil {
		return fmt.Errorf("updating CI trust %s status: %w", id, err)
	}

	log.Printf("INFO CITrust %s reconciled: workspace=%s", id, workspace)
	return nil
}

func (r *CITrustReconciler) handleDelete(ctx context.Context, id string, trust *pb.CITrust) error {
	// TODO(PLAT-DATA-25): delete Keycloak claim mapper, token-exchange
	// permission, and gateway RBAC binding for this trust
	log.Printf("INFO CITrust %s: would delete Keycloak artifacts for gateway=%s (Keycloak integration pending)",
		id, trust.GetGatewayId())

	return nil
}
