package platformGatewayPolicies

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/openshift-online/hypershell/components/api-server/pkg/cedar"
	"github.com/openshift-online/hypershell/components/api-server/pkg/dberr"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
)

// Event sources streamed to the control plane over the gRPC watch
// (PLAT-PGP-12, CTRL-CORE-22).
const (
	policyEventSource   = "PlatformGatewayPolicies"
	settingsEventSource = "PlatformPolicySettings"
)

var policyNameRE = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// PolicyPatch is a partial update of a policy's spec fields.
type PolicyPatch struct {
	Description      *string
	Target           *Target
	EnforcementMode  *string
	UserMessage      *string
	DocumentationUrl *string
	Cedar            *string
}

// Service is the PlatformGatewayPolicy business logic.
type PolicyService interface {
	Get(ctx context.Context, id string) (*PlatformGatewayPolicy, *errors.ServiceError)
	All(ctx context.Context) (PlatformGatewayPolicyList, *errors.ServiceError)
	Create(ctx context.Context, p *PlatformGatewayPolicy) (*PlatformGatewayPolicy, *errors.ServiceError)
	Update(ctx context.Context, id string, patch PolicyPatch) (*PlatformGatewayPolicy, *errors.ServiceError)
	Delete(ctx context.Context, id string) *errors.ServiceError
	UpdateStatus(ctx context.Context, id string, entries []GatewayStatus, removed []string) (*PlatformGatewayPolicy, *errors.ServiceError)

	Settings(ctx context.Context) (*PolicySettings, *errors.ServiceError)
	SetMode(ctx context.Context, mode, actor string) (*PolicySettings, *errors.ServiceError)

	Config() (Config, *errors.ServiceError)
	ValidateCedar(cedarText string) (*cedar.ValidationError, *errors.ServiceError)

	ReportFindings(ctx context.Context, findings []Finding) (int, *errors.ServiceError)
	ListFindings(ctx context.Context, q FindingQuery) ([]Finding, int64, *errors.ServiceError)
	CountFindings(ctx context.Context, q FindingQuery) (FindingCounts, *errors.ServiceError)
	CountFindingsByGateway(ctx context.Context, policyName string, since time.Time) (map[string]int32, *errors.ServiceError)
	PruneFindings(ctx context.Context, now time.Time) (int64, *errors.ServiceError)
	GatewayExists(ctx context.Context, gatewayID string) (bool, *errors.ServiceError)
}

// NewService builds the service. cfgErr is the configuration load error, if
// any: a misconfigured fleet default fails every validating operation closed
// rather than validating against a guessed default.
func NewService(dao Dao, events services.EventService, cfg Config, cfgErr error) PolicyService {
	return &sqlService{dao: dao, events: events, cfg: cfg, cfgErr: cfgErr}
}

type sqlService struct {
	dao    Dao
	events services.EventService
	cfg    Config
	cfgErr error
}

var _ PolicyService = &sqlService{}

func (s *sqlService) Config() (Config, *errors.ServiceError) {
	if s.cfgErr != nil {
		return Config{}, errors.GeneralError("platform policy configuration is invalid: %s", s.cfgErr)
	}
	return s.cfg, nil
}

// ValidateCedar applies the write-time rules (PLAT-PGP-04) against the
// configured fleet defaults. A nil ValidationError means the text is valid.
func (s *sqlService) ValidateCedar(cedarText string) (*cedar.ValidationError, *errors.ServiceError) {
	cfg, svcErr := s.Config()
	if svcErr != nil {
		return nil, svcErr
	}
	err := cedar.ValidatePolicy(cedarText, cfg.Defaults.Normalized)
	if err == nil {
		return nil, nil
	}
	var ve *cedar.ValidationError
	if stderrors.As(err, &ve) {
		return ve, nil
	}
	return nil, errors.GeneralError("validate cedar: %s", err)
}

func (s *sqlService) Get(ctx context.Context, id string) (*PlatformGatewayPolicy, *errors.ServiceError) {
	item, err := s.dao.Get(ctx, id)
	if err != nil {
		return nil, services.HandleGetError("PlatformGatewayPolicy", "id", id, err)
	}
	return item, nil
}

func (s *sqlService) All(ctx context.Context) (PlatformGatewayPolicyList, *errors.ServiceError) {
	items, err := s.dao.All(ctx)
	if err != nil {
		return nil, errors.GeneralError("unable to list PlatformGatewayPolicies: %s", err)
	}
	return items, nil
}

func validateMode(mode string) *errors.ServiceError {
	if mode != ModeAudit && mode != ModeEnforce {
		return errors.Validation("enforcement mode must be audit or enforce, got %q", mode)
	}
	return nil
}

func validateTarget(t Target) *errors.ServiceError {
	for _, id := range append(append([]string{}, t.GatewayIDs...), t.Exclude...) {
		if strings.TrimSpace(id) == "" {
			return errors.Validation("target gateway ids must not be empty")
		}
	}
	if !t.All && len(t.GatewayIDs) == 0 {
		return errors.Validation("target must select all gateways or at least one gateway id")
	}
	return nil
}

func validateDocumentationURL(raw string) *errors.ServiceError {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return errors.Validation("documentation_url must be an http or https URL")
	}
	return nil
}

// validationToServiceError maps Cedar diagnostics to a 400 that carries every
// diagnostic with its line and suggested fix (PLAT-PGP-04.01).
func validationToServiceError(ve *cedar.ValidationError) *errors.ServiceError {
	parts := make([]string, 0, len(ve.Details))
	for _, d := range ve.Details {
		text := d.Message
		if d.Line > 0 {
			text = fmt.Sprintf("line %d: %s", d.Line, text)
		}
		if d.Suggestion != "" {
			text += " (" + d.Suggestion + ")"
		}
		parts = append(parts, text)
	}
	return errors.Validation("cedar policy validation failed: %s", strings.Join(parts, "; "))
}

func (s *sqlService) validateSpec(p *PlatformGatewayPolicy, target Target) *errors.ServiceError {
	if !policyNameRE.MatchString(p.Name) || len(p.Name) > 63 {
		return errors.Validation("name must be a lowercase DNS label (letters, digits, and hyphens, at most 63 characters)")
	}
	if svcErr := validateTarget(target); svcErr != nil {
		return svcErr
	}
	if svcErr := validateMode(p.EnforcementMode); svcErr != nil {
		return svcErr
	}
	if svcErr := validateDocumentationURL(p.DocumentationUrl); svcErr != nil {
		return svcErr
	}
	ve, svcErr := s.ValidateCedar(p.Cedar)
	if svcErr != nil {
		return svcErr
	}
	if ve != nil {
		return validationToServiceError(ve)
	}
	return nil
}

func encodeTarget(t Target) (string, *errors.ServiceError) {
	if t.All {
		t.GatewayIDs = nil
	}
	sort.Strings(t.GatewayIDs)
	sort.Strings(t.Exclude)
	raw, err := json.Marshal(t)
	if err != nil {
		return "", errors.GeneralError("encode target: %s", err)
	}
	return string(raw), nil
}

// DecodeTarget decodes the stored target document.
func DecodeTarget(raw string) (Target, error) {
	if raw == "" {
		return Target{All: true}, nil
	}
	var t Target
	if err := json.Unmarshal([]byte(raw), &t); err != nil {
		return Target{}, fmt.Errorf("decode policy target: %w", err)
	}
	return t, nil
}

// DecodeStatus decodes the stored status document.
func DecodeStatus(raw string) (StoredStatus, error) {
	var st StoredStatus
	if raw == "" || raw == "{}" {
		return st, nil
	}
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		return StoredStatus{}, fmt.Errorf("decode policy status: %w", err)
	}
	return st, nil
}

func conflictForName(err error, name string) *errors.ServiceError {
	if dberr.IsUniqueViolation(err) {
		return errors.Conflict("a PlatformGatewayPolicy named %q already exists", name)
	}
	return nil
}

func (s *sqlService) emit(ctx context.Context, source, id string, t api.EventType) *errors.ServiceError {
	if _, err := s.events.Create(ctx, &api.Event{Source: source, SourceID: id, EventType: t}); err != nil {
		return errors.GeneralError("unable to record %s event: %s", source, err.Error())
	}
	return nil
}

// Create stores a new policy. Whatever the request says, the policy starts in
// audit (PLAT-PGP-01.02) and is never built in.
func (s *sqlService) Create(ctx context.Context, p *PlatformGatewayPolicy) (*PlatformGatewayPolicy, *errors.ServiceError) {
	target, err := DecodeTarget(p.Target)
	if err != nil {
		return nil, errors.Validation("%s", err)
	}
	p.EnforcementMode = ModeAudit
	p.BuiltIn = false
	p.Status = `{}`
	if svcErr := s.validateSpec(p, target); svcErr != nil {
		return nil, svcErr
	}
	encoded, svcErr := encodeTarget(target)
	if svcErr != nil {
		return nil, svcErr
	}
	p.Target = encoded
	item, dbErr := s.dao.Create(ctx, p)
	if dbErr != nil {
		if c := conflictForName(dbErr, p.Name); c != nil {
			return nil, c
		}
		return nil, errors.GeneralError("unable to create PlatformGatewayPolicy: %s", dbErr)
	}
	if svcErr := s.emit(ctx, policyEventSource, item.ID, api.CreateEventType); svcErr != nil {
		return nil, svcErr
	}
	return item, nil
}

// Update applies a spec patch, including an enforcement-mode change
// (PLAT-PGP-06). A built-in policy cannot be changed over the API.
func (s *sqlService) Update(ctx context.Context, id string, patch PolicyPatch) (*PlatformGatewayPolicy, *errors.ServiceError) {
	found, svcErr := s.Get(ctx, id)
	if svcErr != nil {
		return nil, svcErr
	}
	if found.BuiltIn {
		return nil, errors.Conflict("PlatformGatewayPolicy %q is built in and cannot be changed", found.Name)
	}
	target, err := DecodeTarget(found.Target)
	if err != nil {
		return nil, errors.GeneralError("%s", err)
	}
	if patch.Description != nil {
		found.Description = *patch.Description
	}
	if patch.Target != nil {
		target = *patch.Target
	}
	if patch.EnforcementMode != nil {
		found.EnforcementMode = *patch.EnforcementMode
	}
	if patch.UserMessage != nil {
		found.UserMessage = *patch.UserMessage
	}
	if patch.DocumentationUrl != nil {
		found.DocumentationUrl = *patch.DocumentationUrl
	}
	if patch.Cedar != nil {
		found.Cedar = *patch.Cedar
	}
	if svcErr := s.validateSpec(found, target); svcErr != nil {
		return nil, svcErr
	}
	encoded, svcErr := encodeTarget(target)
	if svcErr != nil {
		return nil, svcErr
	}
	found.Target = encoded
	updated, dbErr := s.dao.Replace(ctx, found)
	if dbErr != nil {
		if c := conflictForName(dbErr, found.Name); c != nil {
			return nil, c
		}
		return nil, errors.GeneralError("unable to update PlatformGatewayPolicy: %s", dbErr)
	}
	if svcErr := s.emit(ctx, policyEventSource, updated.ID, api.UpdateEventType); svcErr != nil {
		return nil, svcErr
	}
	return updated, nil
}

// Delete removes a policy. A built-in policy is refused (PLAT-DATA-28.02,
// PLAT-PGP-20.02).
func (s *sqlService) Delete(ctx context.Context, id string) *errors.ServiceError {
	found, svcErr := s.Get(ctx, id)
	if svcErr != nil {
		return svcErr
	}
	if found.BuiltIn {
		return errors.Conflict("PlatformGatewayPolicy %q is built in and cannot be deleted", found.Name)
	}
	if err := s.dao.Delete(ctx, id); err != nil {
		return errors.GeneralError("unable to delete PlatformGatewayPolicy: %s", err)
	}
	return s.emit(ctx, policyEventSource, id, api.DeleteEventType)
}

// UpdateStatus merges the control plane's per-gateway rollout entries into the
// stored status (CTRL-CORE-22.03). It is status-only and emits no watch event,
// so the control plane's own write-back never re-triggers its reconcile.
func (s *sqlService) UpdateStatus(ctx context.Context, id string, entries []GatewayStatus, removed []string) (*PlatformGatewayPolicy, *errors.ServiceError) {
	found, svcErr := s.Get(ctx, id)
	if svcErr != nil {
		return nil, svcErr
	}
	st, err := DecodeStatus(found.Status)
	if err != nil {
		return nil, errors.GeneralError("%s", err)
	}
	if st.Gateways == nil {
		st.Gateways = map[string]GatewayStatus{}
	}
	for _, e := range entries {
		if e.GatewayID == "" {
			return nil, errors.Validation("gateway status entries must name gateway_id")
		}
		switch e.Condition {
		case ConditionApplied, ConditionPending, ConditionUnsupported:
		default:
			return nil, errors.Validation("condition must be Applied, Pending, or Unsupported, got %q", e.Condition)
		}
		st.Gateways[e.GatewayID] = e
	}
	for _, g := range removed {
		delete(st.Gateways, g)
	}
	raw, err := json.Marshal(st)
	if err != nil {
		return nil, errors.GeneralError("encode policy status: %s", err)
	}
	found.Status = string(raw)
	updated, dbErr := s.dao.Replace(ctx, found)
	if dbErr != nil {
		return nil, errors.GeneralError("unable to update PlatformGatewayPolicy status: %s", dbErr)
	}
	return updated, nil
}

func (s *sqlService) Settings(ctx context.Context) (*PolicySettings, *errors.ServiceError) {
	st, err := s.dao.GetSettings(ctx)
	if err != nil {
		return nil, errors.GeneralError("unable to read platform policy settings: %s", err)
	}
	return st, nil
}

// SetMode moves the global toggle (PLAT-PGP-06). The request is captured by the
// framework audit trail (SEC-RBAC-12); the watch event streams the new mode to
// every control plane.
func (s *sqlService) SetMode(ctx context.Context, mode, actor string) (*PolicySettings, *errors.ServiceError) {
	if svcErr := validateMode(mode); svcErr != nil {
		return nil, svcErr
	}
	saved, err := s.dao.SaveSettings(ctx, &PolicySettings{Mode: mode, UpdatedBy: actor, UpdatedAt: time.Now().UTC()})
	if err != nil {
		return nil, errors.GeneralError("unable to save platform policy settings: %s", err)
	}
	if svcErr := s.emit(ctx, settingsEventSource, SettingsID, api.UpdateEventType); svcErr != nil {
		return nil, svcErr
	}
	return saved, nil
}

// ReportFindings validates and stores interceptor findings idempotently
// (PLAT-PGP-15, PLAT-DATA-30).
func (s *sqlService) ReportFindings(ctx context.Context, findings []Finding) (int, *errors.ServiceError) {
	for _, f := range findings {
		if f.ID == "" || f.GatewayID == "" {
			return 0, errors.Validation("every finding needs an id and a gateway_id")
		}
		if f.Outcome != OutcomeWouldReject && f.Outcome != OutcomeWouldAddDefaults {
			return 0, errors.Validation("finding outcome must be would_reject or would_add_defaults, got %q", f.Outcome)
		}
		if f.OccurredAt.IsZero() {
			return 0, errors.Validation("finding %s has no occurred_at", f.ID)
		}
	}
	n, err := s.dao.InsertFindings(ctx, findings)
	if err != nil {
		return n, errors.GeneralError("unable to store policy audit findings: %s", err)
	}
	return n, nil
}

func (s *sqlService) ListFindings(ctx context.Context, q FindingQuery) ([]Finding, int64, *errors.ServiceError) {
	items, total, err := s.dao.ListFindings(ctx, q)
	if err != nil {
		return nil, 0, errors.GeneralError("unable to list policy audit findings: %s", err)
	}
	return items, total, nil
}

func (s *sqlService) CountFindings(ctx context.Context, q FindingQuery) (FindingCounts, *errors.ServiceError) {
	c, err := s.dao.CountFindings(ctx, q)
	if err != nil {
		return FindingCounts{}, errors.GeneralError("unable to count policy audit findings: %s", err)
	}
	return c, nil
}

func (s *sqlService) CountFindingsByGateway(ctx context.Context, policyName string, since time.Time) (map[string]int32, *errors.ServiceError) {
	c, err := s.dao.CountFindingsByGateway(ctx, policyName, since)
	if err != nil {
		return nil, errors.GeneralError("unable to count policy audit findings: %s", err)
	}
	return c, nil
}

// PruneFindings deletes findings older than the configured retention
// (PLAT-DATA-30.02).
func (s *sqlService) PruneFindings(ctx context.Context, now time.Time) (int64, *errors.ServiceError) {
	cfg, svcErr := s.Config()
	if svcErr != nil {
		return 0, svcErr
	}
	n, err := s.dao.PruneFindings(ctx, now.Add(-cfg.Retention))
	if err != nil {
		return 0, errors.GeneralError("unable to prune policy audit findings: %s", err)
	}
	return n, nil
}

func (s *sqlService) GatewayExists(ctx context.Context, gatewayID string) (bool, *errors.ServiceError) {
	ok, err := s.dao.GatewayExists(ctx, gatewayID)
	if err != nil {
		return false, errors.GeneralError("unable to look up gateway: %s", err)
	}
	return ok, nil
}

// EffectiveMode is the stricter of the global toggle and the policy mode
// (PLAT-PGP-06).
func EffectiveMode(globalMode, policyMode string) string {
	if globalMode == ModeEnforce || policyMode == ModeEnforce {
		return ModeEnforce
	}
	return ModeAudit
}
