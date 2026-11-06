// Package defaultWorkspaceMemberships implements DefaultWorkspaceMembership
// (data-model.spec.md, PLAT-DATA-27): HyperShell's desired-state record that a
// subject SHALL be a member, with OpenShell role "user", of its Gateway's
// reserved "default" OpenShell workspace.
//
// The Kind is internal-only: it has no public REST route, no hsctl command, and
// no OpenAPI schema. It is written only by the trusted gateway-access
// (gatewayAccess, GTW-ACC-16/17) and service-account (serviceAccounts,
// OSG-SACC-19) automation hooks through the MembershipService interface in service.go,
// which run in the same process and so call it directly rather than over gRPC
// (mirroring RoleBindingService, which also has no gRPC Create/Delete). It is
// read by the control plane's reconciler (CTRL-CORE-16) over
// DefaultWorkspaceMembershipService's List/Watch RPCs, and by the metrics
// surfaces that report on it (OBS-CRM-14).
package defaultWorkspaceMemberships

import (
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"gorm.io/gorm"
)

// Role is always "user" in version 1 (PLAT-DATA-27.03): automatic membership
// never grants the OpenShell workspace "admin" role.
const RoleUser = "user"

// MemberLimit is OpenShell's per-workspace member cap (GTW-ACC-17, OSG-SACC-19).
// Both automation hooks (gatewayAccess and serviceAccounts) check their
// gateway's live row count against this single source of truth before
// persisting a new membership. It is a var, not a const, so an integration
// test can lower it temporarily and exercise the at-limit rejection path
// without inserting 1,000 real rows.
var MemberLimit int64 = 1000

// Source identifies which automation hook created a row.
const (
	SourceGatewayAccess  = "gateway_access"
	SourceServiceAccount = "service_account"
)

// ReconciliationStatus is control-plane-owned (CTRL-CORE-16).
const (
	StatusPending = "pending"
	StatusReady   = "ready"
	StatusFailed  = "failed"
)

// DefaultWorkspaceMembership binds a subject to the "user" role in a Gateway's
// reserved "default" OpenShell workspace (data-model.spec.md, PLAT-DATA-27).
type DefaultWorkspaceMembership struct {
	api.Meta
	GatewayID            string  `json:"gateway_id" gorm:"index"`
	Subject              string  `json:"subject"`
	Role                 string  `json:"role"`
	Source               string  `json:"source"`
	ReconciliationStatus string  `json:"reconciliation_status"`
	LastReconcileError   *string `json:"last_reconcile_error"`
}

type DefaultWorkspaceMembershipList []*DefaultWorkspaceMembership
type DefaultWorkspaceMembershipIndex map[string]*DefaultWorkspaceMembership

func (l DefaultWorkspaceMembershipList) Index() DefaultWorkspaceMembershipIndex {
	index := DefaultWorkspaceMembershipIndex{}
	for _, o := range l {
		index[o.ID] = o
	}
	return index
}

func (d *DefaultWorkspaceMembership) BeforeCreate(tx *gorm.DB) error {
	if d.ID == "" {
		d.ID = api.NewID()
	}
	if d.Role == "" {
		d.Role = RoleUser
	}
	if d.ReconciliationStatus == "" {
		d.ReconciliationStatus = StatusPending
	}
	return nil
}

// AuditEvent records a create or delete of a DefaultWorkspaceMembership
// (data-model.spec.md, PLAT-DATA-27, "Auditing"). It is always system-managed:
// this plugin is never reached through a public API, so every event is
// attributed to the triggering gateway-access or service-account operation
// (Source), not to a caller's own (absent) workspace-membership permission.
type AuditEvent struct {
	api.Meta
	MembershipID string `json:"membership_id" gorm:"index"`
	GatewayID    string `json:"gateway_id" gorm:"index"`
	Subject      string `json:"subject"`
	Role         string `json:"role"`
	Source       string `json:"source"`
	Action       string `json:"action"`
}

func (AuditEvent) TableName() string {
	return "default_workspace_membership_audit_events"
}

func (d *AuditEvent) BeforeCreate(tx *gorm.DB) error {
	if d.ID == "" {
		d.ID = api.NewID()
	}
	return nil
}
