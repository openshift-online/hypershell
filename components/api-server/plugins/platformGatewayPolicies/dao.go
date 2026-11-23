package platformGatewayPolicies

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
)

// Dao is the data access for policies, the settings singleton, and findings.
type Dao interface {
	Get(ctx context.Context, id string) (*PlatformGatewayPolicy, error)
	Create(ctx context.Context, p *PlatformGatewayPolicy) (*PlatformGatewayPolicy, error)
	Replace(ctx context.Context, p *PlatformGatewayPolicy) (*PlatformGatewayPolicy, error)
	Delete(ctx context.Context, id string) error
	All(ctx context.Context) (PlatformGatewayPolicyList, error)

	GetSettings(ctx context.Context) (*PolicySettings, error)
	SaveSettings(ctx context.Context, s *PolicySettings) (*PolicySettings, error)

	InsertFindings(ctx context.Context, findings []Finding) (int, error)
	ListFindings(ctx context.Context, q FindingQuery) ([]Finding, int64, error)
	CountFindings(ctx context.Context, q FindingQuery) (FindingCounts, error)
	CountFindingsByGateway(ctx context.Context, policyName string, since time.Time) (map[string]int32, error)
	PruneFindings(ctx context.Context, before time.Time) (int64, error)
	GatewayExists(ctx context.Context, gatewayID string) (bool, error)
}

// FindingQuery filters findings. Empty fields do not filter.
type FindingQuery struct {
	GatewayID  string
	PolicyName string
	Outcome    string
	Since      time.Time
	Limit      int
	Offset     int
}

var _ Dao = &sqlDao{}

type sqlDao struct {
	sessionFactory *db.SessionFactory
}

func NewDao(sessionFactory *db.SessionFactory) Dao {
	return &sqlDao{sessionFactory: sessionFactory}
}

func (d *sqlDao) Get(ctx context.Context, id string) (*PlatformGatewayPolicy, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var item PlatformGatewayPolicy
	if err := g2.Take(&item, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (d *sqlDao) Create(ctx context.Context, p *PlatformGatewayPolicy) (*PlatformGatewayPolicy, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Create(p).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return p, nil
}

func (d *sqlDao) Replace(ctx context.Context, p *PlatformGatewayPolicy) (*PlatformGatewayPolicy, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Save(p).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return p, nil
}

func (d *sqlDao) Delete(ctx context.Context, id string) error {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Delete(&PlatformGatewayPolicy{Meta: api.Meta{ID: id}}).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}

func (d *sqlDao) All(ctx context.Context) (PlatformGatewayPolicyList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	items := PlatformGatewayPolicyList{}
	if err := g2.Order("name").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

// GetSettings returns the settings singleton, or the default (audit) when no
// row has been written (PLAT-DATA-29.01).
func (d *sqlDao) GetSettings(ctx context.Context) (*PolicySettings, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var s PolicySettings
	err := g2.Take(&s, "id = ?", SettingsID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &PolicySettings{ID: SettingsID, Mode: ModeAudit}, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// SaveSettings writes the singleton with update-or-create.
func (d *sqlDao) SaveSettings(ctx context.Context, s *PolicySettings) (*PolicySettings, error) {
	g2 := (*d.sessionFactory).New(ctx)
	s.ID = SettingsID
	if err := g2.Exec(`
		INSERT INTO platform_policy_settings (id, mode, updated_by, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET mode = EXCLUDED.mode, updated_by = EXCLUDED.updated_by, updated_at = EXCLUDED.updated_at`,
		s.ID, s.Mode, s.UpdatedBy, s.UpdatedAt).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return s, nil
}

// InsertFindings stores findings idempotently on the sidecar event id
// (PLAT-DATA-30.01) and returns how many rows were newly stored.
func (d *sqlDao) InsertFindings(ctx context.Context, findings []Finding) (int, error) {
	g2 := (*d.sessionFactory).New(ctx)
	stored := 0
	for _, f := range findings {
		summary, err := json.Marshal(f.RequestSummary)
		if err != nil {
			return stored, fmt.Errorf("marshal request summary for finding %s: %w", f.ID, err)
		}
		names := f.PolicyNames
		if names == nil {
			names = []string{}
		}
		res := g2.Exec(`
			INSERT INTO policy_audit_findings
				(id, occurred_at, gateway_id, policy_names, bundle_revision, principal_subject, operation, outcome, request_summary)
			VALUES (?, ?, ?, ?::text[], ?, ?, ?, ?, ?::jsonb)
			ON CONFLICT (id) DO NOTHING`,
			f.ID, f.OccurredAt, f.GatewayID, pq.Array(names), f.BundleRevision, f.PrincipalSubject, f.Operation, f.Outcome, string(summary))
		if res.Error != nil {
			db.MarkForRollback(ctx, res.Error)
			return stored, res.Error
		}
		stored += int(res.RowsAffected)
	}
	return stored, nil
}

func applyFindingFilters(q *gorm.DB, f FindingQuery) *gorm.DB {
	if f.GatewayID != "" {
		q = q.Where("gateway_id = ?", f.GatewayID)
	}
	if f.PolicyName != "" {
		q = q.Where("policy_names @> ?::text[]", pq.Array([]string{f.PolicyName}))
	}
	if f.Outcome != "" {
		q = q.Where("outcome = ?", f.Outcome)
	}
	if !f.Since.IsZero() {
		q = q.Where("occurred_at >= ?", f.Since)
	}
	return q
}

type findingRow struct {
	ID               string
	OccurredAt       time.Time
	GatewayID        string
	PolicyNamesJSON  string
	BundleRevision   string
	PrincipalSubject string
	Operation        string
	Outcome          string
	RequestSummary   string
}

// ListFindings returns matching findings, newest first, and the total count.
func (d *sqlDao) ListFindings(ctx context.Context, f FindingQuery) ([]Finding, int64, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var total int64
	if err := applyFindingFilters(g2.Table("policy_audit_findings"), f).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	q := applyFindingFilters(g2.Table("policy_audit_findings"), f).
		Select(`id, occurred_at, gateway_id, array_to_json(policy_names)::text AS policy_names_json, bundle_revision, principal_subject, operation, outcome, request_summary::text AS request_summary`).
		Order("occurred_at DESC, id")
	if f.Limit > 0 {
		q = q.Limit(f.Limit)
	}
	if f.Offset > 0 {
		q = q.Offset(f.Offset)
	}
	var rows []findingRow
	if err := q.Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]Finding, 0, len(rows))
	for _, r := range rows {
		item := Finding{
			ID:               r.ID,
			OccurredAt:       r.OccurredAt,
			GatewayID:        r.GatewayID,
			BundleRevision:   r.BundleRevision,
			PrincipalSubject: r.PrincipalSubject,
			Operation:        r.Operation,
			Outcome:          r.Outcome,
		}
		if err := json.Unmarshal([]byte(r.PolicyNamesJSON), &item.PolicyNames); err != nil {
			return nil, 0, fmt.Errorf("decode policy names of finding %s: %w", r.ID, err)
		}
		if r.RequestSummary != "" {
			if err := json.Unmarshal([]byte(r.RequestSummary), &item.RequestSummary); err != nil {
				return nil, 0, fmt.Errorf("decode request summary of finding %s: %w", r.ID, err)
			}
		}
		out = append(out, item)
	}
	return out, total, nil
}

// CountFindings returns the would-reject and would-add-defaults counts.
func (d *sqlDao) CountFindings(ctx context.Context, f FindingQuery) (FindingCounts, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var rows []struct {
		Outcome string
		N       int64
	}
	if err := applyFindingFilters(g2.Table("policy_audit_findings"), f).
		Select("outcome, count(*) AS n").Group("outcome").Scan(&rows).Error; err != nil {
		return FindingCounts{}, err
	}
	var c FindingCounts
	for _, r := range rows {
		switch r.Outcome {
		case OutcomeWouldReject:
			c.WouldReject = int32(r.N)
		case OutcomeWouldAddDefaults:
			c.WouldAddDefaults = int32(r.N)
		}
	}
	return c, nil
}

// CountFindingsByGateway returns per-gateway would-reject counts for a policy.
func (d *sqlDao) CountFindingsByGateway(ctx context.Context, policyName string, since time.Time) (map[string]int32, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var rows []struct {
		GatewayID string
		N         int64
	}
	if err := applyFindingFilters(g2.Table("policy_audit_findings"), FindingQuery{PolicyName: policyName, Outcome: OutcomeWouldReject, Since: since}).
		Select("gateway_id, count(*) AS n").Group("gateway_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]int32, len(rows))
	for _, r := range rows {
		out[r.GatewayID] = int32(r.N)
	}
	return out, nil
}

// PruneFindings deletes findings older than before (PLAT-DATA-30.02).
func (d *sqlDao) PruneFindings(ctx context.Context, before time.Time) (int64, error) {
	g2 := (*d.sessionFactory).New(ctx)
	res := g2.Exec(`DELETE FROM policy_audit_findings WHERE occurred_at < ?`, before)
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

// GatewayExists reports whether a live gateway with the id exists. It reads the
// gateways table directly (like the CIProvider reference check) so this plugin
// does not import the gateways plugin.
func (d *sqlDao) GatewayExists(ctx context.Context, gatewayID string) (bool, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var n int64
	if err := g2.Table("gateways").Where("id = ? AND deleted_at IS NULL", gatewayID).Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}
