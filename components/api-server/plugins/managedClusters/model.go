package managedClusters

import (
	"time"

	hypershellapi "github.com/openshift-online/hypershell/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"gorm.io/gorm"
)

type ManagedCluster struct {
	api.Meta
	hypershellapi.TraceMeta
	Name             string     `json:"name"`
	KubeconfigSecret string     `json:"kubeconfig_secret"`
	Status           *string    `json:"status"`
	ApiServerUrl     *string    `json:"api_server_url"`
	OIDCSubject      string     `json:"oidc_subject" gorm:"column:oidc_subject"`
	LastSeenAt       *time.Time `json:"last_seen_at" gorm:"column:last_seen_at"`
	Capabilities     []string   `json:"capabilities" gorm:"serializer:json;type:jsonb"`
	// Health is the spoke-reported on-cluster self-check verdict, written only by
	// the registering control plane through the registration/heartbeat path
	// (PLAT-CLUS-16) and stored verbatim (PLAT-DATA-27). Nil until the first
	// verdict is reported.
	Health *HealthVerdict `json:"health" gorm:"serializer:json;type:jsonb"`
}

// HealthVerdict is the on-cluster self-check verdict the control plane reports,
// stored verbatim on the owning ManagedCluster. Its shape mirrors
// managed-cluster-self-healthcheck.spec.md "Health Verdict Shape" (PLAT-HLTH-03).
type HealthVerdict struct {
	Overall    string            `json:"overall"`
	ObservedAt *time.Time        `json:"observed_at,omitempty"`
	DurationMs int64             `json:"duration_ms"`
	Conditions []HealthCondition `json:"conditions"`
}

// HealthCondition is one per-check condition within a HealthVerdict.
type HealthCondition struct {
	Name             string     `json:"name"`
	Status           string     `json:"status"`
	LastTransitionAt *time.Time `json:"last_transition_at,omitempty"`
	Message          string     `json:"message,omitempty"`
}

// Health verdict rollup values (PLAT-HLTH-03).
const (
	HealthOverallHealthy  = "healthy"
	HealthOverallDegraded = "degraded"
	HealthOverallFailing  = "failing"
)

// Health condition status values (PLAT-HLTH-03).
const (
	HealthStatusPass    = "pass"
	HealthStatusWarn    = "warn"
	HealthStatusFail    = "fail"
	HealthStatusUnknown = "unknown"
)

var validHealthOverall = map[string]bool{
	HealthOverallHealthy:  true,
	HealthOverallDegraded: true,
	HealthOverallFailing:  true,
}

var validHealthConditionStatus = map[string]bool{
	HealthStatusPass:    true,
	HealthStatusWarn:    true,
	HealthStatusFail:    true,
	HealthStatusUnknown: true,
}

// ValidateHealthVerdict rejects a malformed verdict with a 400 so a heartbeat
// carrying one is rejected atomically before any write (PLAT-CLUS-16.03). The hub
// stores the verdict verbatim; it only enforces the bounded enums and that each
// condition carries a name, so a stored verdict is always well-formed for readers.
func ValidateHealthVerdict(v *HealthVerdict) *errors.ServiceError {
	if v == nil {
		return nil
	}
	if !validHealthOverall[v.Overall] {
		return errors.Validation("invalid health.overall %q: expected one of healthy, degraded, failing", v.Overall)
	}
	if v.DurationMs < 0 {
		return errors.Validation("invalid health.duration_ms %d: must not be negative", v.DurationMs)
	}
	seen := map[string]bool{}
	for i := range v.Conditions {
		c := v.Conditions[i]
		if c.Name == "" {
			return errors.Validation("invalid health condition: name must not be empty")
		}
		if seen[c.Name] {
			return errors.Validation("invalid health conditions: duplicate condition name %q", c.Name)
		}
		seen[c.Name] = true
		if !validHealthConditionStatus[c.Status] {
			return errors.Validation("invalid health condition %q status %q: expected one of pass, warn, fail, unknown", c.Name, c.Status)
		}
	}
	return nil
}

type ManagedClusterList []*ManagedCluster
type ManagedClusterIndex map[string]*ManagedCluster

func (l ManagedClusterList) Index() ManagedClusterIndex {
	index := ManagedClusterIndex{}
	for _, o := range l {
		index[o.ID] = o
	}
	return index
}

func (d *ManagedCluster) BeforeCreate(tx *gorm.DB) error {
	d.ID = api.NewID()
	return nil
}

type ManagedClusterPatchRequest struct {
	Name             *string `json:"name,omitempty"`
	KubeconfigSecret *string `json:"kubeconfig_secret,omitempty"`
	Status           *string `json:"status,omitempty"`
	ApiServerUrl     *string `json:"api_server_url,omitempty"`
}
