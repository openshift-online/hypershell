package rbac

import (
	"context"
)

type AccessibleGatewayIDsFunc func(ctx context.Context, userID string) ([]string, error)

type GatewayVisibilityFilter struct {
	fn AccessibleGatewayIDsFunc
}

func NewGatewayVisibilityFilter(fn AccessibleGatewayIDsFunc) *GatewayVisibilityFilter {
	return &GatewayVisibilityFilter{fn: fn}
}

func (f *GatewayVisibilityFilter) AccessibleGatewayIDs(ctx context.Context, userID string) ([]string, error) {
	return f.fn(ctx, userID)
}
