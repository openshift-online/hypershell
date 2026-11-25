package rbac

import "testing"

// TestCanTargetGatewayCluster: gateway:cluster-targeting or platform:admin may choose
// a gateway's cluster; gateway:creator alone may not (SEC-RBAC-34).
// Verifies: SEC-RBAC-34.01, SEC-RBAC-34.02
func TestCanTargetGatewayCluster(t *testing.T) {
	for _, tc := range []struct {
		roles []string
		want  bool
	}{
		{roles: []string{"gateway:cluster-targeting"}, want: true},
		{roles: []string{"platform:admin"}, want: true},
		{roles: []string{"gateway:creator"}, want: false},
		{roles: []string{"gateway:dedicated-route-creator", "gateway:operator-workspace-creator"}, want: false},
		{roles: nil, want: false},
	} {
		bindings := make([]BindingSummary, 0, len(tc.roles))
		for _, r := range tc.roles {
			bindings = append(bindings, BindingSummary{RoleName: r, Scope: "global"})
		}
		if got := CanTargetGatewayCluster(bindings); got != tc.want {
			t.Errorf("CanTargetGatewayCluster(%v) = %t, want %t", tc.roles, got, tc.want)
		}
	}
}
