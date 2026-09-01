package gatewayProfiles

import (
	"context"

	"gorm.io/gorm/clause"

	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
)

type GatewayProfileDao interface {
	Get(ctx context.Context, id string) (*GatewayProfile, error)
	Create(ctx context.Context, gatewayProfile *GatewayProfile) (*GatewayProfile, error)
	Replace(ctx context.Context, gatewayProfile *GatewayProfile) (*GatewayProfile, error)
	Delete(ctx context.Context, id string) error
	FindByIDs(ctx context.Context, ids []string) (GatewayProfileList, error)
	All(ctx context.Context) (GatewayProfileList, error)
	// ExistsByClusterProfileID reports whether any live ManagedCluster references
	// this profile as its default (managed_clusters.profile_id).
	ExistsByClusterProfileID(ctx context.Context, profileID string) (bool, error)
	// ExistsByGatewayProfileID reports whether any live Gateway references this
	// profile (gateways.profile_id).
	ExistsByGatewayProfileID(ctx context.Context, profileID string) (bool, error)
}

var _ GatewayProfileDao = &sqlGatewayProfileDao{}

type sqlGatewayProfileDao struct {
	sessionFactory *db.SessionFactory
}

func NewGatewayProfileDao(sessionFactory *db.SessionFactory) GatewayProfileDao {
	return &sqlGatewayProfileDao{sessionFactory: sessionFactory}
}

func (d *sqlGatewayProfileDao) Get(ctx context.Context, id string) (*GatewayProfile, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var gatewayProfile GatewayProfile
	if err := g2.Take(&gatewayProfile, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &gatewayProfile, nil
}

func (d *sqlGatewayProfileDao) Create(ctx context.Context, gatewayProfile *GatewayProfile) (*GatewayProfile, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Create(gatewayProfile).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return gatewayProfile, nil
}

func (d *sqlGatewayProfileDao) Replace(ctx context.Context, gatewayProfile *GatewayProfile) (*GatewayProfile, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Save(gatewayProfile).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return gatewayProfile, nil
}

func (d *sqlGatewayProfileDao) Delete(ctx context.Context, id string) error {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Delete(&GatewayProfile{Meta: api.Meta{ID: id}}).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}

func (d *sqlGatewayProfileDao) FindByIDs(ctx context.Context, ids []string) (GatewayProfileList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	gatewayProfiles := GatewayProfileList{}
	if err := g2.Where("id in (?)", ids).Find(&gatewayProfiles).Error; err != nil {
		return nil, err
	}
	return gatewayProfiles, nil
}

func (d *sqlGatewayProfileDao) All(ctx context.Context) (GatewayProfileList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	gatewayProfiles := GatewayProfileList{}
	if err := g2.Find(&gatewayProfiles).Error; err != nil {
		return nil, err
	}
	return gatewayProfiles, nil
}

func (d *sqlGatewayProfileDao) ExistsByClusterProfileID(ctx context.Context, profileID string) (bool, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var count int64
	if err := g2.Raw("SELECT COUNT(*) FROM managed_clusters WHERE profile_id = ? AND deleted_at IS NULL", profileID).Scan(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (d *sqlGatewayProfileDao) ExistsByGatewayProfileID(ctx context.Context, profileID string) (bool, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var count int64
	if err := g2.Raw("SELECT COUNT(*) FROM gateways WHERE profile_id = ? AND deleted_at IS NULL", profileID).Scan(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
