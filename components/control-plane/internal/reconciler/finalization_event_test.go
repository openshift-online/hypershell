package reconciler

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// A leaked gateway-owned resource with no automatic recovery path must be
// recorded as a durable, operator-visible Warning Event in the control-plane
// namespace (so it outlives the reaped gateway resources), carrying enough
// identity to act on and never a secret.
func TestRecordIncompleteFinalizationEvent_CreatesDurableWarning(t *testing.T) {
	client := fake.NewSimpleClientset()
	const cpNamespace = "hypershell-control-plane"

	err := recordIncompleteFinalizationEvent(
		context.Background(), client, cpNamespace,
		"gateway-id", "KeycloakClient", "gw-gateway-id",
		"Keycloak unavailable during deletion",
	)
	if err != nil {
		t.Fatalf("recordIncompleteFinalizationEvent: %v", err)
	}

	events, err := client.CoreV1().Events(cpNamespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events.Items) != 1 {
		t.Fatalf("event count = %d, want 1", len(events.Items))
	}
	ev := events.Items[0]
	if ev.Namespace != cpNamespace {
		t.Errorf("event namespace = %q, want %q", ev.Namespace, cpNamespace)
	}
	if ev.Type != corev1.EventTypeWarning {
		t.Errorf("event type = %q, want Warning", ev.Type)
	}
	if ev.Reason != "IncompleteFinalization" {
		t.Errorf("event reason = %q, want IncompleteFinalization", ev.Reason)
	}
	if ev.Source.Component != "hypershell-control-plane" {
		t.Errorf("event component = %q, want hypershell-control-plane", ev.Source.Component)
	}
	if ev.InvolvedObject.Kind != "Gateway" || ev.InvolvedObject.Name != "gateway-id" {
		t.Errorf("involvedObject = %s/%s, want Gateway/gateway-id", ev.InvolvedObject.Kind, ev.InvolvedObject.Name)
	}
	if ev.InvolvedObject.Namespace != cpNamespace {
		t.Errorf("involvedObject namespace = %q, want %q (must match event namespace)", ev.InvolvedObject.Namespace, cpNamespace)
	}
	for _, want := range []string{"gateway-id", "KeycloakClient", "gw-gateway-id", "Keycloak unavailable during deletion"} {
		if !strings.Contains(ev.Message, want) {
			t.Errorf("event message %q missing %q", ev.Message, want)
		}
	}
}

// No control-plane namespace means no durable record is possible. A leaked
// resource with no record is exactly the silent orphan this guards against, so
// the helper must fail closed rather than swallow the leak.
func TestRecordIncompleteFinalizationEvent_FailsClosedWithoutNamespace(t *testing.T) {
	client := fake.NewSimpleClientset()

	err := recordIncompleteFinalizationEvent(
		context.Background(), client, "",
		"gateway-id", "KeycloakClient", "gw-gateway-id", "reason",
	)
	if err == nil {
		t.Fatal("expected an error when no control-plane namespace is configured")
	}

	events, listErr := client.CoreV1().Events("").List(context.Background(), metav1.ListOptions{})
	if listErr != nil {
		t.Fatalf("list events: %v", listErr)
	}
	if len(events.Items) != 0 {
		t.Errorf("no event should be created when the namespace is empty, got %d", len(events.Items))
	}
}
