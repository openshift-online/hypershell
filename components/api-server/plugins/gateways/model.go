package gateways

import (
	"encoding/hex"
	"fmt"

	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"github.com/segmentio/ksuid"
	"gorm.io/gorm"
)

const gatewayNamespacePrefix = "openshell-"

type Gateway struct {
	api.Meta
	Name               string  `json:"name"`
	FleetId            string  `json:"fleet_id"`
	ClusterId          string  `json:"cluster_id"`
	ReleaseId          string  `json:"release_id"`
	DatabaseId         string  `json:"database_id"`
	Namespace          string  `json:"namespace"`
	ExternalDns        *string `json:"external_dns"`
	TlsMode            *string `json:"tls_mode"`
	ServiceType        *string `json:"service_type"`
	Status             *string `json:"status"`
	Phase              *string `json:"phase"`
	Image              *string `json:"image"`
	SupervisorImage    *string `json:"supervisor_image"`
	ServerDnsNames     *string `json:"server_dns_names" gorm:"type:jsonb"`
	RouteAddress       *string `json:"route_address"`
	Oidc               *string `json:"oidc" gorm:"type:jsonb"`
	Route              *string `json:"route" gorm:"type:jsonb"`
	DatabaseConfig     *string `json:"database_config" gorm:"type:jsonb"`
	CredentialDriver   *string `json:"credential_driver" gorm:"type:jsonb"`
	ActiveSandboxCount *int    `json:"active_sandbox_count"`
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
	return nil
}

type GatewayPatchRequest struct {
	Name               *string `json:"name,omitempty"`
	FleetId            *string `json:"fleet_id,omitempty"`
	ClusterId          *string `json:"cluster_id,omitempty"`
	ReleaseId          *string `json:"release_id,omitempty"`
	DatabaseId         *string `json:"database_id,omitempty"`
	ExternalDns        *string `json:"external_dns,omitempty"`
	TlsMode            *string `json:"tls_mode,omitempty"`
	ServiceType        *string `json:"service_type,omitempty"`
	Status             *string `json:"status,omitempty"`
	Phase              *string `json:"phase,omitempty"`
	Image              *string `json:"image,omitempty"`
	SupervisorImage    *string `json:"supervisor_image,omitempty"`
	ServerDnsNames     *string `json:"server_dns_names,omitempty"`
	RouteAddress       *string `json:"route_address,omitempty"`
	Oidc               *string `json:"oidc,omitempty"`
	Route              *string `json:"route,omitempty"`
	DatabaseConfig     *string `json:"database_config,omitempty"`
	CredentialDriver   *string `json:"credential_driver,omitempty"`
}
