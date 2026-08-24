package serviceAccounts

import (
	"context"
	"strings"
	"time"

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
	ConditionalUpdate(context.Context, *OpenShellGatewayServiceAccount, string) (bool, error)
	Get(context.Context, string, string) (*OpenShellGatewayServiceAccount, error)
	List(context.Context, string, ListOptions) ([]OpenShellGatewayServiceAccount, int64, error)
	CountActive(context.Context, string, string) (int64, int64, error)
	ListDueAndTransitional(context.Context, time.Time, int) ([]OpenShellGatewayServiceAccount, error)
	ListDrift(context.Context, time.Time, int) ([]OpenShellGatewayServiceAccount, error)
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

// ConditionalUpdate writes the reconciler-managed columns only when the row's
// persisted status still equals expectedStatus. It returns true when a row was
// updated. This makes the reconciler's terminal write a compare-and-set against
// concurrent foreground mutations even if lifecycle serialization is bypassed,
// so a paused reconciliation can never restore a superseded state.
func (d *sqlServiceAccountDao) ConditionalUpdate(ctx context.Context, account *OpenShellGatewayServiceAccount, expectedStatus string) (bool, error) {
	result := d.session(ctx).Model(&OpenShellGatewayServiceAccount{}).
		Where("id = ? AND status = ?", account.ID, expectedStatus).
		Updates(map[string]any{
			"status":               account.Status,
			"role":                 account.Role,
			"subject":              account.Subject,
			"keycloak_client_uuid": account.KeycloakClientUUID,
			"revoked_at":           account.RevokedAt,
			"last_error":           account.LastError,
		})
	if result.Error != nil {
		db.MarkForRollback(ctx, result.Error)
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
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

// ListDueAndTransitional returns the safety-critical reconciliation work: rows in
// a transitional state (a user- or reconciler-initiated lifecycle change that has
// not converged) plus healthy rows that have reached their expiry. Ordering by
// expiry keeps the soonest-due disablement first so the one-minute revocation
// bound is honored regardless of how many long-lived healthy rows exist. This
// query is served by idx_gateway_service_accounts_reconcile (status, expires_at).
func (d *sqlServiceAccountDao) ListDueAndTransitional(ctx context.Context, now time.Time, limit int) ([]OpenShellGatewayServiceAccount, error) {
	transitional := []string{StatusProvisioning, StatusRevoking, StatusDeleting, StatusError, StatusDegraded}
	var accounts []OpenShellGatewayServiceAccount
	if err := d.session(ctx).
		Where("status IN ? OR (status = ? AND expires_at <= ?)", transitional, StatusReady, now).
		Order("expires_at ASC").Order("updated_at ASC").
		Limit(limit).Find(&accounts).Error; err != nil {
		return nil, err
	}
	return accounts, nil
}

// ListDrift returns ordinary, non-time-critical reconciliation work: healthy rows
// not yet due (convergence drift checks) and terminal rows whose managed client
// should stay disabled. It is paginated separately from the due/transitional scan
// so routine drift can never starve a due disablement.
func (d *sqlServiceAccountDao) ListDrift(ctx context.Context, now time.Time, limit int) ([]OpenShellGatewayServiceAccount, error) {
	terminal := []string{StatusExpired, StatusRevoked}
	var accounts []OpenShellGatewayServiceAccount
	if err := d.session(ctx).
		Where("(status = ? AND expires_at > ?) OR status IN ?", StatusReady, now, terminal).
		Order("updated_at ASC").
		Limit(limit).Find(&accounts).Error; err != nil {
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
