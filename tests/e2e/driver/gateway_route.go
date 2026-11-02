package driver

import (
	"context"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayclient "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned"

	"github.com/openshift-online/hypershell/tests/e2e/harness"
)

const gatewayRouteName = "openshell-gateway"

// waitForGatewayRoute waits for the tenant GRPCRoute and its parent Gateway to
// become ready. Every non-ready observation is retained so a timeout reports the
// Kubernetes condition or API error that kept the route from becoming usable.
func waitForGatewayRoute(ctx context.Context, client gatewayclient.Interface, gw GatewayRef, timeout time.Duration) error {
	lastObservation := "route readiness was not observed"
	err := harness.Poll(ctx, 5*time.Second, timeout, func(ctx context.Context) (bool, error) {
		route, err := client.GatewayV1().GRPCRoutes(gw.Namespace).Get(ctx, gatewayRouteName, metav1.GetOptions{})
		if err != nil {
			lastObservation = fmt.Sprintf("get GRPCRoute %s/%s: %v", gw.Namespace, gatewayRouteName, err)
			return false, nil
		}
		if !grpcRouteAccepted(route) {
			lastObservation = fmt.Sprintf("GRPCRoute %s/%s not accepted: %s", gw.Namespace, gatewayRouteName, grpcRouteAcceptanceObservation(route))
			return false, nil
		}
		if len(route.Spec.ParentRefs) == 0 {
			lastObservation = fmt.Sprintf("GRPCRoute %s/%s has no parentRefs", gw.Namespace, gatewayRouteName)
			return false, nil
		}

		parentObservations := make([]string, 0, len(route.Spec.ParentRefs))
		for _, ref := range route.Spec.ParentRefs {
			ns := gw.Namespace
			if ref.Namespace != nil {
				ns = string(*ref.Namespace)
			}
			name := string(ref.Name)
			parent, err := client.GatewayV1().Gateways(ns).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				parentObservations = append(parentObservations, fmt.Sprintf("get parent Gateway %s/%s: %v", ns, name, err))
				continue
			}
			if gatewayProgrammed(parent) {
				return true, nil
			}
			parentObservations = append(parentObservations, fmt.Sprintf("parent Gateway %s/%s not programmed: %s", ns, name,
				conditionObservation(parent.Status.Conditions, string(gatewayv1.GatewayConditionProgrammed))))
		}
		lastObservation = strings.Join(parentObservations, "; ")
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("wait for GRPCRoute %s/%s readiness (last observation: %s): %w", gw.Namespace, gatewayRouteName, lastObservation, err)
	}
	return nil
}

func grpcRouteAcceptanceObservation(route *gatewayv1.GRPCRoute) string {
	if len(route.Status.Parents) == 0 {
		return "no parent status reported"
	}
	observations := make([]string, 0, len(route.Status.Parents))
	for _, parent := range route.Status.Parents {
		observations = append(observations, fmt.Sprintf("parent=%s controller=%s %s", parent.ParentRef.Name, parent.ControllerName,
			conditionObservation(parent.Conditions, string(gatewayv1.RouteConditionAccepted))))
	}
	return strings.Join(observations, "; ")
}

func conditionObservation(conditions []metav1.Condition, conditionType string) string {
	for _, condition := range conditions {
		if condition.Type == conditionType {
			return fmt.Sprintf("%s=%s reason=%q message=%q observedGeneration=%d", conditionType, condition.Status,
				condition.Reason, condition.Message, condition.ObservedGeneration)
		}
	}
	return fmt.Sprintf("%s condition not reported", conditionType)
}
