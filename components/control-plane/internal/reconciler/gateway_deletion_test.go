package reconciler

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
	kubetesting "k8s.io/client-go/testing"
)

func TestGatewayDeletionWaitsForNamespaceAbsence(t *testing.T) {
	now := metav1.Now()
	client := kubernetesfake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway", DeletionTimestamp: &now, Finalizers: []string{"test/finalizer"}}})
	if err := requireNamespaceAbsent(context.Background(), client, "gateway"); err == nil {
		t.Fatal("terminating namespace was treated as absent")
	}
	if err := client.CoreV1().Namespaces().Delete(context.Background(), "gateway", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := requireNamespaceAbsent(context.Background(), client, "gateway"); err != nil {
		t.Fatal(err)
	}
	client.PrependReactor("get", "namespaces", func(kubetesting.Action) (bool, runtime.Object, error) { return true, nil, errors.New("forbidden") })
	if err := requireNamespaceAbsent(context.Background(), client, "gateway"); err == nil {
		t.Fatal("failed namespace lookup was treated as absent")
	}
}
