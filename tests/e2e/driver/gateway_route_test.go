package driver

import (
	"context"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayfake "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned/fake"

	"github.com/openshift-online/hypershell/tests/e2e/harness"
)

func TestOpenShiftGatewayRouteTimeoutReportsLastObservation(t *testing.T) {
	t.Setenv("E2E_PROVISION_TIMEOUT", "10ms")
	route := &gatewayv1.GRPCRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "openshell-gateway", Namespace: "tenant-a"},
		Spec:       gatewayv1.GRPCRouteSpec{CommonRouteSpec: gatewayv1.CommonRouteSpec{ParentRefs: []gatewayv1.ParentReference{{Name: "shared-gateway"}}}},
		Status: gatewayv1.GRPCRouteStatus{RouteStatus: gatewayv1.RouteStatus{Parents: []gatewayv1.RouteParentStatus{{
			ParentRef:      gatewayv1.ParentReference{Name: "shared-gateway"},
			ControllerName: "example.net/gateway-controller",
			Conditions: []metav1.Condition{{
				Type:    string(gatewayv1.RouteConditionAccepted),
				Status:  metav1.ConditionFalse,
				Reason:  "NoMatchingParent",
				Message: "listener does not allow this route",
			}},
		}}}},
	}
	d := &openshiftDriver{clients: &harness.Clients{Gateway: gatewayfake.NewSimpleClientset(route)}}

	err := d.WaitForGatewayRoute(context.Background(), GatewayRef{Namespace: "tenant-a"})
	if err == nil {
		t.Fatal("WaitForGatewayRoute succeeded for a rejected GRPCRoute")
	}
	for _, want := range []string{"tenant-a/openshell-gateway", "Accepted=False", "NoMatchingParent", "listener does not allow this route"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("WaitForGatewayRoute error %q does not contain %q", err, want)
		}
	}
}
