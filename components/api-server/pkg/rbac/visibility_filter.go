package rbac

import (
	"context"
)

type AccessibleGatewayIDsFunc func(ctx context.Context, userID string) ([]string, error)

type gatewayVisibilityFilter struct {
	fn AccessibleGatewayIDsFunc
}

func NewGatewayVisibilityFilter(fn AccessibleGatewayIDsFunc) *gatewayVisibilityFilter {
	return &gatewayVisibilityFilter{fn: fn}
}

func (f *gatewayVisibilityFilter) AccessibleGatewayIDs(ctx context.Context, userID string) ([]string, error) {
	return f.fn(ctx, userID)
}
