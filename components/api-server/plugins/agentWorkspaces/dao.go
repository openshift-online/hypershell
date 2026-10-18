package agentWorkspaces

import (
	"context"

	"gorm.io/gorm/clause"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
)

type AgentWorkspaceDao interface {
	Get(ctx context.Context, id string) (*AgentWorkspace, error)
	Create(ctx context.Context, agentWorkspace *AgentWorkspace) (*AgentWorkspace, error)
	Replace(ctx context.Context, agentWorkspace *AgentWorkspace) (*AgentWorkspace, error)
	Delete(ctx context.Context, id string) error
	FindByIDs(ctx context.Context, ids []string) (AgentWorkspaceList, error)
	All(ctx context.Context) (AgentWorkspaceList, error)
	// FindByGatewayName returns the workspaces on a gateway that carry a given
	// name. It backs the advisory 409 name-collision check: a name taken on the
	// same gateway by a DIFFERENT agent_runtime_id is a collision, while the same
	// runtime re-declaring its own name is the idempotent no-op.
	FindByGatewayName(ctx context.Context, gatewayID, name string) (AgentWorkspaceList, error)
	// FindByAgentRuntime returns the workspaces owned by an AgentRuntime. It backs
	// the one-workspace-per-runtime invariant (data-model.spec.md "AgentRuntime as
	// a Top-Level Resource").
	FindByAgentRuntime(ctx context.Context, agentRuntimeID string) (AgentWorkspaceList, error)
	// ProviderBindingsForWorkspace, MembershipsForWorkspace, and
	// InferenceRouteForWorkspace read the sibling workspace sub-resource tables
	// directly by workspace_id so the AgentRuntime reconcile payload can embed them
	// without a cross-plugin import cycle (data-model.spec.md "AgentRuntime Reconcile
	// Payload Completeness"). They never read or return secret material - only
	// references (secret_source_id).
	ProviderBindingsForWorkspace(ctx context.Context, workspaceID string) ([]ProviderBindingRow, error)
	MembershipsForWorkspace(ctx context.Context, workspaceID string) ([]MembershipRow, error)
	InferenceRouteForWorkspace(ctx context.Context, workspaceID string) (*InferenceRouteRow, error)
}

// ProviderBindingRow is the subset of a provider_bindings row embedded in the
// AgentRuntime payload. RefreshStrategy and SecretSourceId are references only.
type ProviderBindingRow struct {
	ID              string
	Name            string
	WorkspaceId     string
	ProviderSpecId  string
	SecretSourceId  *string
	RefreshStrategy *string
	Status          *string
}

// MembershipRow is the subset of a workspace_memberships row embedded in the payload.
type MembershipRow struct {
	ID          string
	WorkspaceId string
	Subject     string
	Role        string
}

// InferenceRouteRow is the subset of an inference_routes row embedded in the payload.
type InferenceRouteRow struct {
	ID                string
	WorkspaceId       string
	ProviderBindingId string
	Model             string
}

var _ AgentWorkspaceDao = &sqlAgentWorkspaceDao{}

type sqlAgentWorkspaceDao struct {
	sessionFactory *db.SessionFactory
}

func NewAgentWorkspaceDao(sessionFactory *db.SessionFactory) AgentWorkspaceDao {
	return &sqlAgentWorkspaceDao{sessionFactory: sessionFactory}
}

func (d *sqlAgentWorkspaceDao) Get(ctx context.Context, id string) (*AgentWorkspace, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var item AgentWorkspace
	if err := g2.Take(&item, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (d *sqlAgentWorkspaceDao) Create(ctx context.Context, agentWorkspace *AgentWorkspace) (*AgentWorkspace, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Create(agentWorkspace).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return agentWorkspace, nil
}

func (d *sqlAgentWorkspaceDao) Replace(ctx context.Context, agentWorkspace *AgentWorkspace) (*AgentWorkspace, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Save(agentWorkspace).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return agentWorkspace, nil
}

func (d *sqlAgentWorkspaceDao) Delete(ctx context.Context, id string) error {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Delete(&AgentWorkspace{Meta: api.Meta{ID: id}}).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}

func (d *sqlAgentWorkspaceDao) FindByIDs(ctx context.Context, ids []string) (AgentWorkspaceList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	items := AgentWorkspaceList{}
	if err := g2.Where("id in (?)", ids).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (d *sqlAgentWorkspaceDao) All(ctx context.Context) (AgentWorkspaceList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	items := AgentWorkspaceList{}
	if err := g2.Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (d *sqlAgentWorkspaceDao) FindByGatewayName(ctx context.Context, gatewayID, name string) (AgentWorkspaceList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	items := AgentWorkspaceList{}
	if err := g2.Where("gateway_id = ? AND name = ?", gatewayID, name).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (d *sqlAgentWorkspaceDao) FindByAgentRuntime(ctx context.Context, agentRuntimeID string) (AgentWorkspaceList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	items := AgentWorkspaceList{}
	if err := g2.Where("agent_runtime_id = ?", agentRuntimeID).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (d *sqlAgentWorkspaceDao) ProviderBindingsForWorkspace(ctx context.Context, workspaceID string) ([]ProviderBindingRow, error) {
	g2 := (*d.sessionFactory).New(ctx)
	rows := []ProviderBindingRow{}
	if err := g2.Table("provider_bindings").
		Where("workspace_id = ? AND deleted_at IS NULL", workspaceID).
		Order("id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (d *sqlAgentWorkspaceDao) MembershipsForWorkspace(ctx context.Context, workspaceID string) ([]MembershipRow, error) {
	g2 := (*d.sessionFactory).New(ctx)
	rows := []MembershipRow{}
	if err := g2.Table("workspace_memberships").
		Where("workspace_id = ? AND deleted_at IS NULL", workspaceID).
		Order("id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (d *sqlAgentWorkspaceDao) InferenceRouteForWorkspace(ctx context.Context, workspaceID string) (*InferenceRouteRow, error) {
	g2 := (*d.sessionFactory).New(ctx)
	rows := []InferenceRouteRow{}
	if err := g2.Table("inference_routes").
		Where("workspace_id = ? AND deleted_at IS NULL", workspaceID).
		Order("id").
		Limit(1).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}
