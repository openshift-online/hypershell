package defaultWorkspaceBootstrap

import (
	"context"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
	"gorm.io/gorm/clause"
)

type Dao interface {
	Create(context.Context, *Attempt) error
}

type sqlDao struct {
	sessionFactory *db.SessionFactory
}

func NewDao(sessionFactory *db.SessionFactory) Dao {
	return &sqlDao{sessionFactory: sessionFactory}
}

func (d *sqlDao) Create(ctx context.Context, attempt *Attempt) error {
	session := (*d.sessionFactory).New(ctx)
	if err := session.Omit(clause.Associations).Create(attempt).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}
