package platformGatewayPolicies

import (
	"time"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"gorm.io/gorm"
)

// Enforcement modes (PLAT-PGP-06).
const (
	ModeAudit   = "audit"
	ModeEnforce = "enforce"
)

// Rollout conditions written back by the control plane (CTRL-CORE-22).
const (
	ConditionApplied     = "Applied"
	ConditionPending     = "Pending"
	ConditionUnsupported = "Unsupported"
)

// Finding outcomes (PLAT-DATA-30).
const (
	OutcomeWouldReject      = "would_reject"
	OutcomeWouldAddDefaults = "would_add_defaults"
)

// CanaryPolicyName is the built-in e2e enforcement canary (PLAT-PGP-20).
const CanaryPolicyName = "e2e-enforcement-canary"

// PlatformGatewayPolicy is one platform admission rule set: forbid-only Cedar
// plus user-facing metadata (PLAT-DATA-28). Target and Status are JSON documents
// stored in jsonb columns; the 24-hour finding counts and the validation result
// are computed when the policy is read, never stored.
type PlatformGatewayPolicy struct {
	api.Meta
	Name             string `json:"name"`
	Description      string `json:"description"`
	Target           string `json:"target" gorm:"type:jsonb"`
	EnforcementMode  string `json:"enforcement_mode"`
	UserMessage      string `json:"user_message"`
	DocumentationUrl string `json:"documentation_url"`
	Cedar            string `json:"cedar" gorm:"type:text"`
	BuiltIn          bool   `json:"built_in"`
	Status           string `json:"status" gorm:"type:jsonb"`
}

// TableName pins the clean plural used by the OpenAPI path and the ext gate.
func (PlatformGatewayPolicy) TableName() string { return "platform_gateway_policies" }

type PlatformGatewayPolicyList []*PlatformGatewayPolicy

func (d *PlatformGatewayPolicy) BeforeCreate(tx *gorm.DB) error {
	if d.ID == "" {
		d.ID = api.NewID()
	}
	if d.Target == "" {
		d.Target = `{"all":true}`
	}
	if d.Status == "" {
		d.Status = `{}`
	}
	return nil
}

// Target is the decoded target selector (PLAT-PGP-01).
type Target struct {
	All        bool     `json:"all"`
	GatewayIDs []string `json:"gateway_ids,omitempty"`
	Exclude    []string `json:"exclude,omitempty"`
}

// Applies reports whether the target selects gatewayID.
func (t Target) Applies(gatewayID string) bool {
	for _, ex := range t.Exclude {
		if ex == gatewayID {
			return false
		}
	}
	if t.All {
		return true
	}
	for _, id := range t.GatewayIDs {
		if id == gatewayID {
			return true
		}
	}
	return false
}

// GatewayStatus is one gateway's rollout entry in the stored status.
type GatewayStatus struct {
	GatewayID             string     `json:"gateway_id"`
	Condition             string     `json:"condition"`
	Revision              string     `json:"revision,omitempty"`
	AppliedRevision       string     `json:"applied_revision,omitempty"`
	EffectiveMode         string     `json:"effective_mode,omitempty"`
	Message               string     `json:"message,omitempty"`
	NonCompliantSandboxes []string   `json:"non_compliant_sandboxes,omitempty"`
	ObservedAt            *time.Time `json:"observed_at,omitempty"`
}

// StoredStatus is the jsonb status document: per-gateway rollout keyed by id.
type StoredStatus struct {
	Gateways map[string]GatewayStatus `json:"gateways,omitempty"`
}

// PolicySettings is the platform-wide singleton holding the global toggle
// (PLAT-DATA-29) and the accepted fleet defaults (PLAT-PGP-08). The row id is
// always SettingsID; when no row exists the mode is audit. The defaults columns
// are server-populated: they hold the last defaults the api-server accepted
// from its configuration, and DefaultsRefusal explains why a configured change
// is refused. Empty DefaultCPURequest means no defaults were accepted yet.
type PolicySettings struct {
	ID                   string `gorm:"primaryKey"`
	Mode                 string `gorm:"not null"`
	UpdatedBy            string
	UpdatedAt            time.Time
	DefaultCPURequest    string     `gorm:"column:default_cpu_request"`
	DefaultCPULimit      string     `gorm:"column:default_cpu_limit"`
	DefaultMemoryRequest string     `gorm:"column:default_memory_request"`
	DefaultMemoryLimit   string     `gorm:"column:default_memory_limit"`
	DefaultsRefusal      string     `gorm:"column:defaults_refusal"`
	DefaultsAcceptedAt   *time.Time `gorm:"column:defaults_accepted_at"`
}

// AcceptedDefaults returns the stored accepted defaults as quantities, and
// false when none were accepted yet.
func (s *PolicySettings) AcceptedDefaults() (FleetDefaults, bool) {
	if s == nil || s.DefaultCPURequest == "" {
		return FleetDefaults{}, false
	}
	return FleetDefaults{
		CPURequest:    s.DefaultCPURequest,
		CPULimit:      s.DefaultCPULimit,
		MemoryRequest: s.DefaultMemoryRequest,
		MemoryLimit:   s.DefaultMemoryLimit,
	}, true
}

// SettingsID is the primary key of the settings singleton row.
const SettingsID = "global"

func (PolicySettings) TableName() string { return "platform_policy_settings" }

// Finding is one interceptor audit event (PLAT-DATA-30).
type Finding struct {
	ID               string
	OccurredAt       time.Time
	GatewayID        string
	PolicyNames      []string
	BundleRevision   string
	PrincipalSubject string
	Operation        string
	Outcome          string
	RequestSummary   map[string]int64
}

// FindingCounts are the would-reject and would-add-defaults counts.
type FindingCounts struct {
	WouldReject      int32
	WouldAddDefaults int32
}
