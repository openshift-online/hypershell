package sandboxTemplates

import (
	"context"

	"gorm.io/gorm/clause"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
)

type SandboxTemplateDao interface {
	Get(ctx context.Context, id string) (*SandboxTemplate, error)
	Create(ctx context.Context, sandboxTemplate *SandboxTemplate) (*SandboxTemplate, error)
	Replace(ctx context.Context, sandboxTemplate *SandboxTemplate) (*SandboxTemplate, error)
	Delete(ctx context.Context, id string) error
	FindByIDs(ctx context.Context, ids []string) (SandboxTemplateList, error)
	All(ctx context.Context) (SandboxTemplateList, error)
}

var _ SandboxTemplateDao = &sqlSandboxTemplateDao{}

type sqlSandboxTemplateDao struct {
	sessionFactory *db.SessionFactory
}

func NewSandboxTemplateDao(sessionFactory *db.SessionFactory) SandboxTemplateDao {
	return &sqlSandboxTemplateDao{sessionFactory: sessionFactory}
}

func (d *sqlSandboxTemplateDao) Get(ctx context.Context, id string) (*SandboxTemplate, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var item SandboxTemplate
	if err := g2.Take(&item, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (d *sqlSandboxTemplateDao) Create(ctx context.Context, sandboxTemplate *SandboxTemplate) (*SandboxTemplate, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Create(sandboxTemplate).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return sandboxTemplate, nil
}

func (d *sqlSandboxTemplateDao) Replace(ctx context.Context, sandboxTemplate *SandboxTemplate) (*SandboxTemplate, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Save(sandboxTemplate).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return sandboxTemplate, nil
}

func (d *sqlSandboxTemplateDao) Delete(ctx context.Context, id string) error {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Delete(&SandboxTemplate{Meta: api.Meta{ID: id}}).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}

func (d *sqlSandboxTemplateDao) FindByIDs(ctx context.Context, ids []string) (SandboxTemplateList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	items := SandboxTemplateList{}
	if err := g2.Where("id in (?)", ids).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (d *sqlSandboxTemplateDao) All(ctx context.Context) (SandboxTemplateList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	items := SandboxTemplateList{}
	if err := g2.Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}
