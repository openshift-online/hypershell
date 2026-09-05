package reconciler

import (
	"context"
	"fmt"
	"testing"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
	"google.golang.org/grpc"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

func legacyGatewayNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				gateway.ManagedByLabel: gateway.ManagedByValue,
				gateway.ManagedLabel:   gateway.ManagedLabelValue,
			},
		},
	}
}

func foreignGatewayNamespace(name, instance string) *corev1.Namespace {
	ns := legacyGatewayNamespace(name)
	ns.Labels[gateway.InstanceLabel] = instance
	return ns
}

func gatewayWithNamespace(id, namespace string) *pb.Gateway {
	return &pb.Gateway{
		Metadata:  &pb.ObjectReference{Id: id},
		Namespace: namespace,
	}
}

// singlePageGatewayClient returns a fake that serves the whole inventory on the
// first page. listAllGateways stops once a short page is returned, so a slice
// smaller than the page size terminates paging after page 1.
func singlePageGatewayClient(gws []*pb.Gateway) *fakeGatewayClient {
	return &fakeGatewayClient{
		listFn: func(ctx context.Context, in *pb.ListGatewaysRequest, opts ...grpc.CallOption) (*pb.ListGatewaysResponse, error) {
			meta := &pb.ListMeta{Page: in.Page, Size: in.Size, Total: int32(len(gws))}
			if in.Page > 1 {
				return &pb.ListGatewaysResponse{Metadata: meta}, nil
			}
			return &pb.ListGatewaysResponse{Items: gws, Metadata: meta}, nil
		},
	}
}

func TestBackfillInstanceLabels(t *testing.T) {
	ctx := context.Background()

	t.Run("labels legacy namespaces of live gateways and spares foreign ones", func(t *testing.T) {
		client := fake.NewSimpleClientset(
			legacyGatewayNamespace("openshell-a"),
			legacyGatewayNamespace("openshell-b"),
			foreignGatewayNamespace("openshell-c", "stage"),
		)
		gwClient := singlePageGatewayClient([]*pb.Gateway{
			gatewayWithNamespace("id-a", "openshell-a"),
			gatewayWithNamespace("id-b", "openshell-b"),
			gatewayWithNamespace("id-c", "openshell-c"),
			gatewayWithNamespace("id-missing", "openshell-not-present"),
			gatewayWithNamespace("id-empty", ""),
		})

		labeled, err := BackfillInstanceLabels(ctx, client, gwClient, "hypershell", "")
		if err != nil {
			t.Fatalf("BackfillInstanceLabels() error = %v", err)
		}
		if labeled != 2 {
			t.Errorf("labeled = %d, want 2", labeled)
		}
		for _, name := range []string{"openshell-a", "openshell-b"} {
			got, err := client.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				t.Fatalf("get namespace %s: %v", name, err)
			}
			if got.Labels[gateway.InstanceLabel] != "hypershell" {
				t.Errorf("%s instance label = %q, want hypershell", name, got.Labels[gateway.InstanceLabel])
			}
		}
		foreign, err := client.CoreV1().Namespaces().Get(ctx, "openshell-c", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get namespace openshell-c: %v", err)
		}
		if foreign.Labels[gateway.InstanceLabel] != "stage" {
			t.Errorf("foreign namespace relabeled to %q, want stage", foreign.Labels[gateway.InstanceLabel])
		}
	})

	t.Run("aborts when the gateway list fails", func(t *testing.T) {
		client := fake.NewSimpleClientset(legacyGatewayNamespace("openshell-a"))
		gwClient := &fakeGatewayClient{
			listFn: func(ctx context.Context, in *pb.ListGatewaysRequest, opts ...grpc.CallOption) (*pb.ListGatewaysResponse, error) {
				return nil, fmt.Errorf("boom")
			},
		}
		if _, err := BackfillInstanceLabels(ctx, client, gwClient, "hypershell", ""); err == nil {
			t.Fatalf("BackfillInstanceLabels() error = nil, want list failure")
		}
		got, err := client.CoreV1().Namespaces().Get(ctx, "openshell-a", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get namespace: %v", err)
		}
		if _, ok := got.Labels[gateway.InstanceLabel]; ok {
			t.Errorf("namespace labeled despite list failure: %v", got.Labels)
		}
	})

	t.Run("collects a per-namespace failure and continues", func(t *testing.T) {
		client := fake.NewSimpleClientset(
			legacyGatewayNamespace("openshell-a"),
			legacyGatewayNamespace("openshell-b"),
		)
		// Fail the Get for openshell-a only; openshell-b must still be labeled.
		client.PrependReactor("get", "namespaces", func(action ktesting.Action) (bool, runtime.Object, error) {
			if ga, ok := action.(ktesting.GetAction); ok && ga.GetName() == "openshell-a" {
				return true, nil, fmt.Errorf("transient get error")
			}
			return false, nil, nil
		})
		gwClient := singlePageGatewayClient([]*pb.Gateway{
			gatewayWithNamespace("id-a", "openshell-a"),
			gatewayWithNamespace("id-b", "openshell-b"),
		})

		labeled, err := BackfillInstanceLabels(ctx, client, gwClient, "hypershell", "")
		if err == nil {
			t.Fatalf("BackfillInstanceLabels() error = nil, want a collected per-namespace error")
		}
		if labeled != 1 {
			t.Errorf("labeled = %d, want 1 (openshell-b still labeled)", labeled)
		}
		got, err := client.CoreV1().Namespaces().Get(ctx, "openshell-b", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get namespace openshell-b: %v", err)
		}
		if got.Labels[gateway.InstanceLabel] != "hypershell" {
			t.Errorf("openshell-b instance label = %q, want hypershell", got.Labels[gateway.InstanceLabel])
		}
	})

	t.Run("refuses an empty instance identity", func(t *testing.T) {
		client := fake.NewSimpleClientset()
		gwClient := singlePageGatewayClient(nil)
		if _, err := BackfillInstanceLabels(ctx, client, gwClient, "", ""); err == nil {
			t.Fatalf("BackfillInstanceLabels() error = nil, want empty instance error")
		}
	})

	t.Run("scopes the gateway listing to this cluster", func(t *testing.T) {
		client := fake.NewSimpleClientset()
		var gotClusterID *string
		gwClient := &fakeGatewayClient{
			listFn: func(ctx context.Context, in *pb.ListGatewaysRequest, opts ...grpc.CallOption) (*pb.ListGatewaysResponse, error) {
				gotClusterID = in.ClusterId
				return &pb.ListGatewaysResponse{Metadata: &pb.ListMeta{Total: 0}}, nil
			},
		}
		if _, err := BackfillInstanceLabels(ctx, client, gwClient, "hypershell", "mc1"); err != nil {
			t.Fatalf("BackfillInstanceLabels() error = %v", err)
		}
		if gotClusterID == nil || *gotClusterID != "mc1" {
			t.Fatalf("cluster_id = %v, want \"mc1\" (backfill must scope to its own cluster)", gotClusterID)
		}
	})
}
