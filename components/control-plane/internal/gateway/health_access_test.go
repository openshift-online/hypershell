package gateway

import (
	"context"
	"io"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"net/http"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestReconcileGatewayHealthAccess(t *testing.T) {
	const namespace = "openshell-test"
	clientset := k8sfake.NewSimpleClientset()

	if err := ReconcileGatewayHealthAccess(context.Background(), clientset, namespace); err != nil {
		t.Fatalf("ReconcileGatewayHealthAccess() error = %v", err)
	}

	service, err := clientset.CoreV1().Services(namespace).Get(context.Background(), GatewayHealthServiceName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get Service: %v", err)
	}
	if len(service.Spec.Ports) != 1 {
		t.Fatalf("Service ports = %#v, want one port", service.Spec.Ports)
	}
	healthPort := service.Spec.Ports[0]
	if healthPort.Name != gatewayHealthPortName || healthPort.Port != GatewayHealthPort || healthPort.TargetPort.String() != gatewayHealthPortName {
		t.Fatalf("health Service port = %#v", healthPort)
	}

	mutations := mutationCount(clientset.Actions())
	if err := ReconcileGatewayHealthAccess(context.Background(), clientset, namespace); err != nil {
		t.Fatalf("second ReconcileGatewayHealthAccess() error = %v", err)
	}
	if got := mutationCount(clientset.Actions()); got != mutations {
		t.Fatalf("second pass mutation count = %d, want %d", got, mutations)
	}
}

func TestReconcileGatewayHealthAccessRepairsDrift(t *testing.T) {
	const namespace = "openshell-test"
	clientset := k8sfake.NewSimpleClientset()
	if err := ReconcileGatewayHealthAccess(context.Background(), clientset, namespace); err != nil {
		t.Fatalf("initial ReconcileGatewayHealthAccess() error = %v", err)
	}

	service, err := clientset.CoreV1().Services(namespace).Get(context.Background(), GatewayHealthServiceName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get health Service: %v", err)
	}
	service.Spec.Selector = map[string]string{"drifted": "true"}
	service.Spec.Ports[0].Port = 9999
	service.Spec.Type = corev1.ServiceTypeLoadBalancer
	service.Spec.ExternalIPs = []string{"192.0.2.1"}
	loadBalancerClass := "example.com/external"
	service.Spec.LoadBalancerClass = &loadBalancerClass
	service.Labels = nil
	if _, err := clientset.CoreV1().Services(namespace).Update(context.Background(), service, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("drift health Service: %v", err)
	}

	if err := ReconcileGatewayHealthAccess(context.Background(), clientset, namespace); err != nil {
		t.Fatalf("repair ReconcileGatewayHealthAccess() error = %v", err)
	}

	service, err = clientset.CoreV1().Services(namespace).Get(context.Background(), GatewayHealthServiceName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get repaired health Service: %v", err)
	}
	if service.Spec.Type != corev1.ServiceTypeClusterIP || len(service.Spec.ExternalIPs) != 0 || service.Spec.LoadBalancerClass != nil {
		t.Fatalf("repaired health Service exposure = %#v", service.Spec)
	}
	if service.Spec.Ports[0].Port != GatewayHealthPort || service.Spec.Selector["app.kubernetes.io/instance"] != GatewayDeploymentName {
		t.Fatalf("repaired health Service = %#v", service.Spec)
	}
	if service.Labels["hypershell.redhat.io/managed"] != "true" {
		t.Fatalf("repaired health Service labels = %#v", service.Labels)
	}
}

func mutationCount(actions []k8stesting.Action) int {
	count := 0
	for _, action := range actions {
		switch action.GetVerb() {
		case "create", "delete", "patch", "update":
			count++
		}
	}
	return count
}

type healthAccessTransport func(*http.Request) (*http.Response, error)

func (f healthAccessTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestHealthAccessOperationsHaveBoundedTimeout(t *testing.T) {
	transport := healthAccessTransport(func(r *http.Request) (*http.Response, error) {
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) > gatewayHealthAccessTimeout {
			t.Fatal("access request has no bounded deadline")
		}
		_ = deadline
		body := `{"apiVersion":"v1","kind":"Service","metadata":{"name":"openshell-gateway-health"}}`
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	clientset, err := kubernetes.NewForConfig(&rest.Config{Host: "http://kubernetes.test", Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	if err := ReconcileGatewayHealthAccess(context.Background(), clientset, "gateway"); err != nil {
		t.Fatal(err)
	}
}
