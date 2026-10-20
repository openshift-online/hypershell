package agentRuntimes

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
)

type AgentRuntimeDao interface {
	Get(ctx context.Context, id string) (*AgentRuntime, error)
	// GetUnscoped returns an AgentRuntime including soft-deleted rows. The watch
	// handler uses it on a delete event, whose row is already soft-deleted, to
	// resolve the deleted runtime's effective cluster for stream scoping.
	GetUnscoped(ctx context.Context, id string) (*AgentRuntime, error)
	Create(ctx context.Context, agentRuntime *AgentRuntime) (*AgentRuntime, error)
	Replace(ctx context.Context, agentRuntime *AgentRuntime) (*AgentRuntime, error)
	// UpdateStatus writes only the control-plane-owned status columns for id,
	// leaving every spec column untouched so a concurrent user PATCH is not
	// clobbered by the status write-back. It returns the reloaded row.
	UpdateStatus(ctx context.Context, id string, status string, statusMessage *string) (*AgentRuntime, error)
	Delete(ctx context.Context, id string) error
	FindByIDs(ctx context.Context, ids []string) (AgentRuntimeList, error)
	All(ctx context.Context) (AgentRuntimeList, error)
	// RepositoryExists reports whether a live Repository with the given id exists.
	RepositoryExists(ctx context.Context, repositoryID string) (bool, error)
}

var _ AgentRuntimeDao = &sqlAgentRuntimeDao{}

type sqlAgentRuntimeDao struct {
	sessionFactory *db.SessionFactory
}

func NewAgentRuntimeDao(sessionFactory *db.SessionFactory) AgentRuntimeDao {
	return &sqlAgentRuntimeDao{sessionFactory: sessionFactory}
}

func (d *sqlAgentRuntimeDao) Get(ctx context.Context, id string) (*AgentRuntime, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var agentRuntime AgentRuntime
	if err := g2.Take(&agentRuntime, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &agentRuntime, nil
}

func (d *sqlAgentRuntimeDao) GetUnscoped(ctx context.Context, id string) (*AgentRuntime, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var agentRuntime AgentRuntime
	if err := g2.Unscoped().Take(&agentRuntime, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &agentRuntime, nil
}

func (d *sqlAgentRuntimeDao) Create(ctx context.Context, agentRuntime *AgentRuntime) (*AgentRuntime, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Create(agentRuntime).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return agentRuntime, nil
}

func (d *sqlAgentRuntimeDao) Replace(ctx context.Context, agentRuntime *AgentRuntime) (*AgentRuntime, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Save(agentRuntime).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return agentRuntime, nil
}

// UpdateStatus writes only the status and status_message columns through a scoped
// UPDATE rather than a full-row Save. This closes the lost-update race in the
// status write-back: a full-row Save re-persists every column from a snapshot read
// before the write, so a user PATCH that lands between that read and the write is
// silently clobbered. Restricting the UPDATE to the two control-plane-owned columns
// means a concurrent spec PATCH to any other column survives untouched. GORM stamps
// updated_at on the UPDATE, so watchers still observe a new version.
func (d *sqlAgentRuntimeDao) UpdateStatus(ctx context.Context, id string, status string, statusMessage *string) (*AgentRuntime, error) {
	g2 := (*d.sessionFactory).New(ctx)
	res := g2.Model(&AgentRuntime{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":         status,
			"status_message": statusMessage,
		})
	if res.Error != nil {
		db.MarkForRollback(ctx, res.Error)
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return d.Get(ctx, id)
}

func (d *sqlAgentRuntimeDao) Delete(ctx context.Context, id string) error {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Delete(&AgentRuntime{Meta: api.Meta{ID: id}}).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}

func (d *sqlAgentRuntimeDao) FindByIDs(ctx context.Context, ids []string) (AgentRuntimeList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	agentRuntimes := AgentRuntimeList{}
	if err := g2.Where("id in (?)", ids).Find(&agentRuntimes).Error; err != nil {
		return nil, err
	}
	return agentRuntimes, nil
}

func (d *sqlAgentRuntimeDao) All(ctx context.Context) (AgentRuntimeList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	agentRuntimes := AgentRuntimeList{}
	if err := g2.Find(&agentRuntimes).Error; err != nil {
		return nil, err
	}
	return agentRuntimes, nil
}

func (d *sqlAgentRuntimeDao) RepositoryExists(ctx context.Context, repositoryID string) (bool, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var count int64
	if err := g2.Table("repositories").
		Where("id = ? AND deleted_at IS NULL", repositoryID).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
