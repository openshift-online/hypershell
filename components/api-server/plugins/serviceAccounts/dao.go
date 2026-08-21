package serviceAccounts

import (
	"context"
	"strings"

	"github.com/openshift-online/rh-trex-ai/pkg/db"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ListOptions struct {
	Page          int
	Size          int
	Status        string
	Search        string
	Sort          string
	Order         string
	CreatorUserID string
}

type ServiceAccountDao interface {
	ActiveNameExists(context.Context, string, string) (bool, error)
	Create(context.Context, *OpenShellGatewayServiceAccount) error
	Update(context.Context, *OpenShellGatewayServiceAccount) error
	Get(context.Context, string, string) (*OpenShellGatewayServiceAccount, error)
	List(context.Context, string, ListOptions) ([]OpenShellGatewayServiceAccount, int64, error)
	CountActive(context.Context, string, string) (int64, int64, error)
	ListReconcilable(context.Context, int) ([]OpenShellGatewayServiceAccount, error)
	SoftDelete(context.Context, *OpenShellGatewayServiceAccount) error
	CreateAudit(context.Context, *AuditEvent) error
}

type sqlServiceAccountDao struct{ sessionFactory *db.SessionFactory }

func NewServiceAccountDao(factory *db.SessionFactory) ServiceAccountDao {
	return &sqlServiceAccountDao{sessionFactory: factory}
}

func (d *sqlServiceAccountDao) session(ctx context.Context) *gorm.DB {
	return (*d.sessionFactory).New(ctx)
}

func (d *sqlServiceAccountDao) ActiveNameExists(ctx context.Context, gatewayID, name string) (bool, error) {
	var count int64
	err := d.session(ctx).Model(&OpenShellGatewayServiceAccount{}).
		Where("gateway_id = ? AND lower(name) = lower(?) AND status NOT IN ?", gatewayID, name, []string{StatusExpired, StatusRevoked}).
		Count(&count).Error
	return count > 0, err
}

func (d *sqlServiceAccountDao) Create(ctx context.Context, account *OpenShellGatewayServiceAccount) error {
	if err := d.session(ctx).Omit(clause.Associations).Create(account).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}

func (d *sqlServiceAccountDao) Update(ctx context.Context, account *OpenShellGatewayServiceAccount) error {
	if err := d.session(ctx).Omit(clause.Associations).Save(account).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}

func (d *sqlServiceAccountDao) Get(ctx context.Context, gatewayID, id string) (*OpenShellGatewayServiceAccount, error) {
	var account OpenShellGatewayServiceAccount
	if err := d.session(ctx).Where("gateway_id = ? AND id = ?", gatewayID, id).Take(&account).Error; err != nil {
		return nil, err
	}
	return &account, nil
}

func (d *sqlServiceAccountDao) List(ctx context.Context, gatewayID string, options ListOptions) ([]OpenShellGatewayServiceAccount, int64, error) {
	query := d.session(ctx).Model(&OpenShellGatewayServiceAccount{}).Where("gateway_id = ?", gatewayID)
	if options.CreatorUserID != "" {
		query = query.Where("created_by_user_id = ?", options.CreatorUserID)
	}
	if options.Status != "" {
		query = query.Where("status = ?", options.Status)
	}
	if options.Search != "" {
		escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(options.Search)
		pattern := "%" + escaped + "%"
		query = query.Where(`(name ILIKE ? ESCAPE '\' OR keycloak_client_id ILIKE ? ESCAPE '\' OR subject ILIKE ? ESCAPE '\')`, pattern, pattern, pattern)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	sortColumn := map[string]string{
		"name": "name", "role": "role", "status": "status", "expires_at": "expires_at", "created_at": "created_at",
	}[options.Sort]
	if sortColumn == "" {
		sortColumn = "created_at"
	}
	direction := "DESC"
	if options.Order == "asc" {
		direction = "ASC"
	}
	var accounts []OpenShellGatewayServiceAccount
	offset := (options.Page - 1) * options.Size
	if err := query.Order(sortColumn + " " + direction).Order("id " + direction).Offset(offset).Limit(options.Size).Find(&accounts).Error; err != nil {
		return nil, 0, err
	}
	return accounts, total, nil
}

func (d *sqlServiceAccountDao) CountActive(ctx context.Context, gatewayID, creatorUserID string) (int64, int64, error) {
	var gatewayCount int64
	if err := d.session(ctx).Model(&OpenShellGatewayServiceAccount{}).
		Where("gateway_id = ? AND status NOT IN ?", gatewayID, []string{StatusExpired, StatusRevoked}).
		Count(&gatewayCount).Error; err != nil {
		return 0, 0, err
	}
	var creatorCount int64
	if err := d.session(ctx).Model(&OpenShellGatewayServiceAccount{}).
		Where("gateway_id = ? AND created_by_user_id = ? AND status NOT IN ?", gatewayID, creatorUserID, []string{StatusExpired, StatusRevoked}).
		Count(&creatorCount).Error; err != nil {
		return 0, 0, err
	}
	return gatewayCount, creatorCount, nil
}

func (d *sqlServiceAccountDao) ListReconcilable(ctx context.Context, limit int) ([]OpenShellGatewayServiceAccount, error) {
	var accounts []OpenShellGatewayServiceAccount
	if err := d.session(ctx).Order("updated_at ASC").Limit(limit).Find(&accounts).Error; err != nil {
		return nil, err
	}
	return accounts, nil
}

func (d *sqlServiceAccountDao) SoftDelete(ctx context.Context, account *OpenShellGatewayServiceAccount) error {
	if err := d.session(ctx).Delete(account).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}

func (d *sqlServiceAccountDao) CreateAudit(ctx context.Context, event *AuditEvent) error {
	return d.session(ctx).Omit(clause.Associations).Create(event).Error
}
