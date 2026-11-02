package inferenceRoutes

import (
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"gorm.io/gorm"
)

type InferenceRoute struct {
	api.Meta
	// Name is the queryable find-by-name key (SDLC-CONF-13). It defaults to a
	// stable identity derived from workspace_id/provider_binding_id/model when a
	// client does not supply one. A PARTIAL UNIQUE index (live rows only) makes
	// find-by-name resolve exactly one live row (an ambiguous name would make hsctl
	// apply resolve the wrong record) while leaving a soft-deleted row's lingering
	// name free, so a reconcile can re-apply a deleted route without a 409. The index
	// is created by migrationAddName, NOT by a model-level uniqueIndex tag: a full
	// tag would make CreateTable build a full index covering deleted rows
	// (SDLC-CONF-13, FINDING 1).
	Name              string `json:"name"`
	WorkspaceId       string `json:"workspace_id"`
	ProviderBindingId string `json:"provider_binding_id"`
	Model             string `json:"model"`
	// RepositoryId is the ownership mark set by the control-plane declarative
	// import reconciler when this resource originates from a Repository's
	// .hypershell import. It is nil for resources created directly through the
	// REST API or hsctl apply, which are never pruned by the import. It is
	// read-only in the client contract (set server-side, never by a client).
	RepositoryId *string `json:"repository_id"`
}

type InferenceRouteList []*InferenceRoute

func (d *InferenceRoute) BeforeCreate(tx *gorm.DB) error {
	if d.ID == "" {
		d.ID = api.NewID()
	}
	// Guarantee a find-by-name key on every row regardless of entry path (REST or
	// gRPC import). When a client does not supply a name, derive a stable identity
	// from the composite identity columns, mirroring the migration backfill for
	// pre-existing rows (SDLC-CONF-13).
	if d.Name == "" {
		d.Name = DeriveName(d.WorkspaceId, d.ProviderBindingId, d.Model)
	}
	return nil
}

// DeriveName builds the default find-by-name key for an InferenceRoute from its
// composite identity (workspace_id/provider_binding_id/model). The migration
// backfill derives the same shape for pre-existing rows.
func DeriveName(workspaceID, providerBindingID, model string) string {
	return workspaceID + "/" + providerBindingID + "/" + model
}
