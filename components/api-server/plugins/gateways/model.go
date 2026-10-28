package gateways

import (
	"encoding/hex"
	"fmt"

	hypershellapi "github.com/openshift-online/hypershell/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/segmentio/ksuid"
	"gorm.io/gorm"
)

const gatewayNamespacePrefix = "openshell-"

type Gateway struct {
	api.Meta
	hypershellapi.TraceMeta
	Name      string `json:"name"`
	ClusterId string `json:"cluster_id"`
	Namespace string `json:"namespace"`
	// Placement is the persisted capability-label placement selector for a
	// late-bound gateway, stored as a JSON-encoded GatewayPlacementIntent
	// ({"selector":[...]} or {"mode":"local-kind"}). NULL for a gateway pinned with
	// an explicit ClusterId. The API server resolves it to a concrete ClusterId at
	// create time and re-evaluates it for late binding. See data-model.spec.md,
	// "Requirement: Gateway Placement and Late Binding".
	Placement       *string `json:"placement" gorm:"type:jsonb"`
	ExternalDns     *string `json:"external_dns"`
	TlsMode         *string `json:"tls_mode"`
	ServiceType     *string `json:"service_type"`
	Status          *string `json:"status"`
	Phase           *string `json:"phase"`
	Image           *string `json:"image"`
	SupervisorImage *string `json:"supervisor_image"`
	ServerDnsNames  *string `json:"server_dns_names" gorm:"type:jsonb"`
	RouteAddress    *string `json:"route_address"`
	ConsoleAddress  *string `json:"console_address"`
	GatewayVersion  *string `json:"gateway_version"`
	Oidc            *string `json:"oidc" gorm:"type:jsonb"`
	Route           *string `json:"route" gorm:"type:jsonb"`
	// DedicatedRoute records a create-time request for the dedicated per-gateway
	// route and wildcard certificate path. User-writable at create only, and only
	// for a caller holding the gateway:dedicated-route-creator entitlement
	// (PLAT-DATA-23); the API server translates it into the platform-managed
	// route.certificate opt-in the control plane keys on. NULL on gateways created
	// without the request, which means "not requested" (same as false).
	DedicatedRoute   *bool   `json:"dedicated_route"`
	CredentialDriver *string `json:"credential_driver" gorm:"type:jsonb"`
	// WorkspaceMode selects how the OpenShell Kubernetes driver maps workspaces
	// to namespaces (shared/managed/operator). Platform-managed (PLAT-DATA-23):
	// the API server defaults it to "managed" at create; no caller can set or
	// change it (the service funnel enforces the vocabulary and immutability).
	// NULL on gateways created before the field existed; those keep the chart's
	// shared default (openshell-gateway.spec.md, "OSG-CORE-18 -- Gateway Workspace
	// Mode").
	WorkspaceMode          *string `json:"workspace_mode"`
	ActiveSandboxCount     *int    `json:"active_sandbox_count"`
	ProvisioningConditions *string `json:"provisioning_conditions" gorm:"type:jsonb"`
	Generation             int64   `json:"generation" gorm:"not null;default:1"`
	ObservedGeneration     int64   `json:"observed_generation" gorm:"not null;default:0"`
	// RepositoryId is the control-plane import ownership mark: the Repository that
	// owns this gateway through the declarative .hypershell import. NULL for gateways
	// created directly through the REST API or hsctl apply. Read-only in the client
	// contract; set server-side from the import binding, never from a client body.
	RepositoryId *string `json:"repository_id"`
}

type GatewayList []*Gateway
type GatewayIndex map[string]*Gateway

func (l GatewayList) Index() GatewayIndex {
	index := GatewayIndex{}
	for _, o := range l {
		index[o.ID] = o
	}
	return index
}

func (d *Gateway) BeforeCreate(tx *gorm.DB) error {
	d.ID = api.NewID()

	id, err := ksuid.Parse(d.ID)
	if err != nil {
		return fmt.Errorf("parse generated gateway ID: %w", err)
	}
	d.Namespace = gatewayNamespacePrefix + hex.EncodeToString(id.Payload()[:8])
	d.Generation = 1
	d.ObservedGeneration = 0
	return nil
}
