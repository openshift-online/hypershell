package authz_test

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/test"
)

// importMD returns a context carrying the reconciled-Repository import binding, the
// trusted gRPC metadata the server reads CITrust ownership and the SEC-RBAC-26
// gateway-ownership gate from.
func importMD(repositoryID string) context.Context {
	return metadata.AppendToOutgoingContext(context.Background(), rbac.ImportRepositoryMetadataKey, repositoryID)
}

// registerImportRepository registers a Repository over REST (as the owner) and
// returns its id, so an import write can name a real reconciled Repository.
func registerImportRepository(t *testing.T, client *openapi.APIClient, owner principal) string {
	t.Helper()
	repo, _, err := client.DefaultAPI.CreateRepository(owner.rest).
		Repository(*openapi.NewRepository("repo-"+short(), "https://github.com/openshift-online/"+short(), "github")).Execute()
	Expect(err).NotTo(HaveOccurred(), "register repository: %s", test.APIErrorReason(err))
	return repo.GetId()
}

// TestCITrustDeclarativeImportRoundTrip is the DB-backed importer round-trip for a
// CITrust over the production driver (SDLC-CONF-08.03, PLAT-CITR-10, PLAT-DATA-25).
// It drives the real control-plane gRPC import write path: a registered control
// plane, carrying a reconciled-Repository binding, upserts a CITrust referencing a
// CIProvider and a Repository-owned Gateway. The server SHALL set repository_id
// ownership from the trusted metadata, leave the row queryable, enforce the
// SEC-RBAC-26 gateway-ownership tenancy gate, and scope prune per-Repository.
//
// Verifies: SDLC-CONF-08.03, PLAT-CITR-10, SEC-RBAC-26.01, SEC-RBAC-26.02, SEC-RBAC-09.09
func TestCITrustDeclarativeImportRoundTrip(t *testing.T) {
	RegisterTestingT(t)
	h, client := test.RegisterIntegration(t)
	h.StartControllersServer()

	owner := newPrincipal(t, h, "citrust-import-"+short(), realmRoles("platform:admin", "gateway:creator"))

	// A registered control plane (spoke) is the only identity whose import-marked
	// CITrust write is authorized over gRPC (SEC-RBAC-09.09).
	spoke := registerSpoke(t, h, client)
	gateways := pb.NewGatewayServiceClient(spoke.conn)
	trusts := pb.NewCITrustServiceClient(spoke.conn)

	// Two Repositories: R1 imports the trust; R2 owns a sibling gateway R1 may not
	// author a trust on.
	repo1 := registerImportRepository(t, client, owner)
	repo2 := registerImportRepository(t, client, owner)

	// A CIProvider is a platform-wide registration authored over HTTP (not imported).
	cp, _, err := client.DefaultAPI.CreateCIProvider(owner.rest).
		CIProvider(*openapi.NewCIProvider("github", "https://token.actions.githubusercontent.com/"+short(), "https://example.com/jwks", "hypershell")).Execute()
	Expect(err).NotTo(HaveOccurred(), "create ci provider: %s", test.APIErrorReason(err))
	ciProviderID := cp.GetId()

	// A Gateway owned by R1, created the way the import authors one: a control-plane
	// write carrying R1's reconciled-Repository binding sets repository_id = R1.
	gw1, err := gateways.CreateGateway(importMD(repo1), &pb.CreateGatewayRequest{
		Name:      "gw-r1-" + short(),
		ClusterId: spoke.clusterID,
	})
	Expect(err).NotTo(HaveOccurred(), "import create gateway owned by R1: %v", err)
	Expect(gw1.GetGateway().GetRepositoryId()).To(Equal(repo1), "gateway must be owned by R1")
	gatewayR1 := gw1.GetGateway().GetMetadata().GetId()

	// A sibling Gateway owned by R2.
	gw2, err := gateways.CreateGateway(importMD(repo2), &pb.CreateGatewayRequest{
		Name:      "gw-r2-" + short(),
		ClusterId: spoke.clusterID,
	})
	Expect(err).NotTo(HaveOccurred(), "import create gateway owned by R2: %v", err)
	gatewayR2 := gw2.GetGateway().GetMetadata().GetId()

	// --- SEC-RBAC-26.01: import authors a CITrust on its own R1-owned gateway ---
	name := "github-actions-" + short()
	created, err := trusts.CreateCITrust(importMD(repo1), &pb.CreateCITrustRequest{
		Name:              &name,
		CiProviderId:      ciProviderID,
		GatewayId:         gatewayR1,
		Role:              "openshell-user",
		SubjectRepository: openapi.PtrString("openshift-online/hypershell"),
		SubjectRef:        openapi.PtrString("refs/heads/main"),
	})
	Expect(err).NotTo(HaveOccurred(), "import create CITrust on R1-owned gateway: %v", err)
	trustID := created.GetCiTrust().GetMetadata().GetId()
	Expect(created.GetCiTrust().GetRepositoryId()).To(Equal(repo1),
		"the server must set repository_id ownership from the trusted metadata")
	Expect(created.GetCiTrust().GetName()).To(Equal(name), "the manifest metadata.name must be preserved")

	// Queryable over the production driver: Get returns the owned row.
	got, err := trusts.GetCITrust(importMD(repo1), &pb.GetCITrustRequest{Id: trustID})
	Expect(err).NotTo(HaveOccurred(), "get imported CITrust: %v", err)
	Expect(got.GetCiTrust().GetRepositoryId()).To(Equal(repo1))

	// --- FINDING 1: import is DESIRED STATE. An import update that OMITS a
	// previously-set subject field CLEARS it (replace semantics), unlike the human
	// sparse PATCH where nil means "unchanged" (SEC-RBAC-22). The manifest now declares
	// only subject.repository, so subject_ref must be cleared over the production driver.
	updated, err := trusts.UpdateCITrust(importMD(repo1), &pb.UpdateCITrustRequest{
		Id:                trustID,
		Role:              openapi.PtrString("openshell-user"),
		SubjectRepository: openapi.PtrString("openshift-online/hypershell"),
		// SubjectRef intentionally omitted -> import replace must clear it.
	})
	Expect(err).NotTo(HaveOccurred(), "import update clearing subject_ref: %v", err)
	Expect(updated.GetCiTrust().GetSubjectRef()).To(BeEmpty(),
		"an import update that omits subject_ref must clear it (desired-state replace, FINDING 1)")
	Expect(updated.GetCiTrust().GetSubjectRepository()).To(Equal("openshift-online/hypershell"),
		"the declared subject_repository must be retained")

	// --- SEC-RBAC-26.02: import may not author a CITrust on a gateway it does not own ---
	_, err = trusts.CreateCITrust(importMD(repo1), &pb.CreateCITrustRequest{
		Name:         openapi.PtrString("cross-tenant-" + short()),
		CiProviderId: ciProviderID,
		GatewayId:    gatewayR2, // owned by R2
		Role:         "openshell-user",
	})
	Expect(status.Code(err)).To(Equal(codes.PermissionDenied),
		"import CITrust on an R2-owned gateway must be refused: %v", err)

	// --- Per-Repository prune: R2's import may not delete R1's trust (EnforceImportOwnership) ---
	_, err = trusts.DeleteCITrust(importMD(repo2), &pb.DeleteCITrustRequest{Id: trustID})
	Expect(status.Code(err)).To(Equal(codes.PermissionDenied),
		"a different repository's import must not prune R1's trust: %v", err)

	// --- Prune: R1's import removes the trust it owns ---
	_, err = trusts.DeleteCITrust(importMD(repo1), &pb.DeleteCITrustRequest{Id: trustID})
	Expect(err).NotTo(HaveOccurred(), "R1 import prune of its own trust: %v", err)

	_, err = trusts.GetCITrust(importMD(repo1), &pb.GetCITrustRequest{Id: trustID})
	Expect(status.Code(err)).To(Equal(codes.NotFound), "the pruned trust must be gone: %v", err)
}
