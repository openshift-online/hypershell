package inferenceRoutes

import (
	"context"

	"gorm.io/gorm/clause"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
)

type InferenceRouteDao interface {
	Get(ctx context.Context, id string) (*InferenceRoute, error)
	Create(ctx context.Context, inferenceRoute *InferenceRoute) (*InferenceRoute, error)
	Delete(ctx context.Context, id string) error
	FindByIDs(ctx context.Context, ids []string) (InferenceRouteList, error)
	All(ctx context.Context) (InferenceRouteList, error)
}

var _ InferenceRouteDao = &sqlInferenceRouteDao{}

type sqlInferenceRouteDao struct {
	sessionFactory *db.SessionFactory
}

func NewInferenceRouteDao(sessionFactory *db.SessionFactory) InferenceRouteDao {
	return &sqlInferenceRouteDao{sessionFactory: sessionFactory}
}

func (d *sqlInferenceRouteDao) Get(ctx context.Context, id string) (*InferenceRoute, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var item InferenceRoute
	if err := g2.Take(&item, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (d *sqlInferenceRouteDao) Create(ctx context.Context, inferenceRoute *InferenceRoute) (*InferenceRoute, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Create(inferenceRoute).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return inferenceRoute, nil
}

func (d *sqlInferenceRouteDao) Delete(ctx context.Context, id string) error {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Delete(&InferenceRoute{Meta: api.Meta{ID: id}}).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}

func (d *sqlInferenceRouteDao) FindByIDs(ctx context.Context, ids []string) (InferenceRouteList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	items := InferenceRouteList{}
	if err := g2.Where("id in (?)", ids).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (d *sqlInferenceRouteDao) All(ctx context.Context) (InferenceRouteList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	items := InferenceRouteList{}
	if err := g2.Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}
