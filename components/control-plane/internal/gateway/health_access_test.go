package gateway

import (
	"context"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestReconcileGatewayHealthAccess(t *testing.T) {
	const (
		namespace             = "openshell-test"
		controlPlaneNamespace = "hypershell"
	)
	clientset := k8sfake.NewSimpleClientset()

	if err := ReconcileGatewayHealthAccess(context.Background(), clientset, namespace, controlPlaneNamespace, false); err != nil {
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

	policy, err := clientset.NetworkingV1().NetworkPolicies(namespace).Get(context.Background(), gatewayHealthPolicyName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get NetworkPolicy: %v", err)
	}
	if got := policy.Spec.Ingress[0].From[0].NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"]; got != controlPlaneNamespace {
		t.Fatalf("control plane namespace selector = %q, want %q", got, controlPlaneNamespace)
	}
	if got := policy.Spec.Ingress[0].From[0].PodSelector.MatchLabels["app"]; got != "hypershell-controller" {
		t.Fatalf("controller selector = %q, want hypershell-controller", got)
	}
	if got := policy.Spec.Ingress[0].Ports[0].Port.IntVal; got != GatewayHealthPort {
		t.Fatalf("NetworkPolicy port = %d, want %d", got, GatewayHealthPort)
	}

	mutations := mutationCount(clientset.Actions())
	if err := ReconcileGatewayHealthAccess(context.Background(), clientset, namespace, controlPlaneNamespace, false); err != nil {
		t.Fatalf("second ReconcileGatewayHealthAccess() error = %v", err)
	}
	if got := mutationCount(clientset.Actions()); got != mutations {
		t.Fatalf("second pass mutation count = %d, want %d", got, mutations)
	}
}

func TestReconcileGatewayHealthAccessSkipsNetworkPolicy(t *testing.T) {
	const namespace = "openshell-test"
	clientset := k8sfake.NewSimpleClientset()

	if err := ReconcileGatewayHealthAccess(context.Background(), clientset, namespace, "", true); err != nil {
		t.Fatalf("ReconcileGatewayHealthAccess() error = %v", err)
	}
	for _, action := range clientset.Actions() {
		if action.GetResource().Resource == "networkpolicies" {
			t.Fatalf("network policy action = %#v, want none", action)
		}
	}
	if _, err := clientset.CoreV1().Services(namespace).Get(context.Background(), GatewayHealthServiceName, metav1.GetOptions{}); err != nil {
		t.Fatalf("get health Service: %v", err)
	}
}

func TestReconcileGatewayHealthAccessRepairsDrift(t *testing.T) {
	const (
		namespace             = "openshell-test"
		controlPlaneNamespace = "hypershell"
	)
	clientset := k8sfake.NewSimpleClientset()
	if err := ReconcileGatewayHealthAccess(context.Background(), clientset, namespace, controlPlaneNamespace, false); err != nil {
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

	policy, err := clientset.NetworkingV1().NetworkPolicies(namespace).Get(context.Background(), gatewayHealthPolicyName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get health NetworkPolicy: %v", err)
	}
	policy.Spec.Ingress = nil
	policy.Labels = nil
	if _, err := clientset.NetworkingV1().NetworkPolicies(namespace).Update(context.Background(), policy, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("drift health NetworkPolicy: %v", err)
	}

	if err := ReconcileGatewayHealthAccess(context.Background(), clientset, namespace, controlPlaneNamespace, false); err != nil {
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

	policy, err = clientset.NetworkingV1().NetworkPolicies(namespace).Get(context.Background(), gatewayHealthPolicyName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get repaired health NetworkPolicy: %v", err)
	}
	if len(policy.Spec.Ingress) != 1 || policy.Labels["hypershell.redhat.io/managed"] != "true" {
		t.Fatalf("repaired health NetworkPolicy = %#v", policy)
	}
}

func TestReconcileGatewayHealthAccessValidatesBeforeMutation(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()

	if err := ReconcileGatewayHealthAccess(context.Background(), clientset, "openshell-test", "", false); err == nil {
		t.Fatal("ReconcileGatewayHealthAccess() error = nil, want control plane namespace error")
	}
	if got := mutationCount(clientset.Actions()); got != 0 {
		t.Fatalf("mutation count = %d, want 0", got)
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

func TestReconcileGatewayHealthAccessUpgradesExistingPolicies(t *testing.T) {
	ctx := context.Background()
	const namespace = "openshell-existing"
	grpcPort, healthPort, namedHealth := intstr.FromInt32(8080), intstr.FromInt32(8081), intstr.FromString("health")
	peer := networkingv1.NetworkPolicyPeer{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"old-peer": "preserve"}}}
	names := []string{"openshell-gateway-allow-sandbox", "openshell-gateway-allow-sandbox-v2", "openshell-gateway-allow-router", "user-policy"}
	client := k8sfake.NewSimpleClientset()
	for _, name := range names {
		_, err := client.NetworkingV1().NetworkPolicies(namespace).Create(ctx, &networkingv1.NetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Annotations: map[string]string{"preserve": "true"}},
			Spec: networkingv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app.kubernetes.io/name": "openshell"}},
				PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress},
				Ingress: []networkingv1.NetworkPolicyIngressRule{
					{From: []networkingv1.NetworkPolicyPeer{peer}, Ports: []networkingv1.NetworkPolicyPort{{Port: &grpcPort}, {Port: &healthPort}}},
					{From: []networkingv1.NetworkPolicyPeer{peer}, Ports: []networkingv1.NetworkPolicyPort{{Port: &namedHealth}}},
				},
				Egress: []networkingv1.NetworkPolicyEgressRule{{Ports: []networkingv1.NetworkPolicyPort{{Port: &healthPort}}}},
			},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
	}
	// A concurrent writer must cause a retry, not prevent the upgrade.
	conflicted := false
	client.PrependReactor("update", "networkpolicies", func(action k8stesting.Action) (bool, runtime.Object, error) {
		obj := action.(k8stesting.UpdateAction).GetObject().(*networkingv1.NetworkPolicy)
		if obj.Name == names[0] && !conflicted {
			conflicted = true
			return true, nil, k8serrors.NewConflict(schema.GroupResource{Resource: "networkpolicies"}, obj.Name, nil)
		}
		return false, nil, nil
	})
	if err := ReconcileGatewayHealthAccess(ctx, client, namespace, "hypershell-system", false); err != nil {
		t.Fatal(err)
	}
	if !conflicted {
		t.Fatal("the conflict retry was not exercised")
	}
	for _, name := range names {
		got, err := client.NetworkingV1().NetworkPolicies(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if name == "user-policy" {
			if len(got.Spec.Ingress) != 2 || len(got.Spec.Ingress[0].Ports) != 2 {
				t.Fatal("changed a user policy")
			}
			continue
		}
		if len(got.Spec.Ingress) != 1 || len(got.Spec.Ingress[0].Ports) != 1 || *got.Spec.Ingress[0].Ports[0].Port != grpcPort {
			t.Fatalf("%s retains health access or grants all ports: %#v", name, got.Spec.Ingress)
		}
		if !reflect.DeepEqual(got.Spec.Ingress[0].From, []networkingv1.NetworkPolicyPeer{peer}) || got.Annotations["preserve"] != "true" || len(got.Spec.Egress) != 1 {
			t.Fatalf("%s lost unrelated fields: %#v", name, got)
		}
	}
	mutations := mutationCount(client.Actions())
	if err := ReconcileGatewayHealthAccess(ctx, client, namespace, "hypershell-system", false); err != nil {
		t.Fatal(err)
	}
	if got := mutationCount(client.Actions()); got != mutations {
		t.Fatalf("repeat pass wrote resources: %d != %d", got, mutations)
	}
}
