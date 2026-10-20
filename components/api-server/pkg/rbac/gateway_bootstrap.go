package rbac

import (
	"context"
)

type GatewayOwnerBindingCreator interface {
	CreateGatewayOwnerBinding(ctx context.Context, userID string, gatewayID string) error
}

type gatewayBootstrapper struct {
	rbCreator GatewayOwnerBindingCreator
}

func NewGatewayBootstrapper(rbCreator GatewayOwnerBindingCreator) *gatewayBootstrapper {
	return &gatewayBootstrapper{rbCreator: rbCreator}
}

func (b *gatewayBootstrapper) CreateOwnerBinding(ctx context.Context, userID string, gatewayID string) error {
	return b.rbCreator.CreateGatewayOwnerBinding(ctx, userID, gatewayID)
}
