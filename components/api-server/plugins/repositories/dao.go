package repositories

import (
	"context"

	"gorm.io/gorm/clause"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
)

type RepositoryDao interface {
	Get(ctx context.Context, id string) (*Repository, error)
	Create(ctx context.Context, repository *Repository) (*Repository, error)
	Replace(ctx context.Context, repository *Repository) (*Repository, error)
	Delete(ctx context.Context, id string) error
	FindByIDs(ctx context.Context, ids []string) (RepositoryList, error)
	All(ctx context.Context) (RepositoryList, error)
	// SecretSourceExists reports whether a live SecretSource with the given id exists.
	SecretSourceExists(ctx context.Context, secretSourceID string) (bool, error)
	// ReferencingAgentRuntimeIDs returns the ids of live AgentRuntimes that
	// reference the given repository via repository_id.
	ReferencingAgentRuntimeIDs(ctx context.Context, repositoryID string) ([]string, error)
}

var _ RepositoryDao = &sqlRepositoryDao{}

type sqlRepositoryDao struct {
	sessionFactory *db.SessionFactory
}

func NewRepositoryDao(sessionFactory *db.SessionFactory) RepositoryDao {
	return &sqlRepositoryDao{sessionFactory: sessionFactory}
}

func (d *sqlRepositoryDao) Get(ctx context.Context, id string) (*Repository, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var item Repository
	if err := g2.Take(&item, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (d *sqlRepositoryDao) Create(ctx context.Context, repository *Repository) (*Repository, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Create(repository).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return repository, nil
}

func (d *sqlRepositoryDao) Replace(ctx context.Context, repository *Repository) (*Repository, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Save(repository).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return repository, nil
}

func (d *sqlRepositoryDao) Delete(ctx context.Context, id string) error {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Delete(&Repository{Meta: api.Meta{ID: id}}).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}

func (d *sqlRepositoryDao) FindByIDs(ctx context.Context, ids []string) (RepositoryList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	items := RepositoryList{}
	if err := g2.Where("id in (?)", ids).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (d *sqlRepositoryDao) All(ctx context.Context) (RepositoryList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	items := RepositoryList{}
	if err := g2.Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (d *sqlRepositoryDao) SecretSourceExists(ctx context.Context, secretSourceID string) (bool, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var count int64
	if err := g2.Table("secret_sources").
		Where("id = ? AND deleted_at IS NULL", secretSourceID).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (d *sqlRepositoryDao) ReferencingAgentRuntimeIDs(ctx context.Context, repositoryID string) ([]string, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var ids []string
	if err := g2.Table("agent_runtimes").
		Where("repository_id = ? AND deleted_at IS NULL", repositoryID).
		Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}
