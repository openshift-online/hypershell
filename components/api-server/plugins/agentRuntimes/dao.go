package agentRuntimes

import (
	"context"

	"gorm.io/gorm/clause"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
)

type AgentRuntimeDao interface {
	Get(ctx context.Context, id string) (*AgentRuntime, error)
	Create(ctx context.Context, agentRuntime *AgentRuntime) (*AgentRuntime, error)
	Replace(ctx context.Context, agentRuntime *AgentRuntime) (*AgentRuntime, error)
	Delete(ctx context.Context, id string) error
	FindByIDs(ctx context.Context, ids []string) (AgentRuntimeList, error)
	All(ctx context.Context) (AgentRuntimeList, error)
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
