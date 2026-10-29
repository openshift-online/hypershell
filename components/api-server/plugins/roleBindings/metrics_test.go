package roleBindings

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

type gatewayOwnerUsersDao struct {
	count int64
}

func (d gatewayOwnerUsersDao) Get(context.Context, string) (*RoleBinding, error) { return nil, nil }
func (d gatewayOwnerUsersDao) GetUnscoped(context.Context, string) (*RoleBinding, error) {
	return nil, nil
}
func (d gatewayOwnerUsersDao) Create(context.Context, *RoleBinding) (*RoleBinding, error) {
	return nil, nil
}
func (d gatewayOwnerUsersDao) Update(context.Context, *RoleBinding) (*RoleBinding, error) {
	return nil, nil
}
func (d gatewayOwnerUsersDao) Delete(context.Context, string) error { return nil }
func (d gatewayOwnerUsersDao) FindByUserID(context.Context, string) (RoleBindingList, error) {
	return nil, nil
}
func (d gatewayOwnerUsersDao) FindByGatewayID(context.Context, string) (RoleBindingList, error) {
	return nil, nil
}
func (d gatewayOwnerUsersDao) FindByIDs(context.Context, []string) (RoleBindingList, error) {
	return nil, nil
}
func (d gatewayOwnerUsersDao) FindGatewayIDsByUserID(context.Context, string) ([]string, error) {
	return nil, nil
}
func (d gatewayOwnerUsersDao) FindOwnerUsernamesByGatewayIDs(context.Context, []string) (map[string]string, error) {
	return nil, nil
}
func (d gatewayOwnerUsersDao) All(context.Context) (RoleBindingList, error) {
	return nil, nil
}
func (d gatewayOwnerUsersDao) CountGatewayOwnerUsers(context.Context) (int64, error) {
	return d.count, nil
}
func (d gatewayOwnerUsersDao) CountOwnersByGatewayID(context.Context, string) (int64, error) {
	return 0, nil
}

func TestGatewayOwnerUsersCollector(t *testing.T) {
	collector := newGatewayOwnerUsersCollector(gatewayOwnerUsersDao{count: 7})

	if value := testutil.ToFloat64(collector); value != 7 {
		t.Fatalf("gateway owner users = %v, want 7", value)
	}
}
