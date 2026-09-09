package reconciler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
	"google.golang.org/grpc"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func TestImagePullAccessFailureBlocksHealthyGateway(t *testing.T) {
	t.Setenv("GATEWAY_SERVER_TLS_CLUSTER_ISSUER", "")
	t.Setenv("GATEWAY_SANDBOX_IMAGE_PULL_ROLES", `[{"namespace":"images","role":"runner-pull"}]`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body string
		switch {
		case strings.HasSuffix(r.URL.Path, "/deployments/openshell-gateway"):
			body = `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"openshell-gateway"},"spec":{"replicas":1},"status":{"readyReplicas":1}}`
		case r.URL.Path == "/api/v1/namespaces/gateway":
			w.WriteHeader(http.StatusForbidden)
			body = `{"apiVersion":"v1","kind":"Status","status":"Failure","reason":"Forbidden","code":403}`
		case r.URL.Path == "/apis/rbac.authorization.k8s.io/v1/rolebindings":
			body = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"RoleBindingList","items":[]}`
		default:
			t.Errorf("unexpected Kubernetes request: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if _, err := io.WriteString(w, body); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	clientset, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	h := newHealthRec(nil, time.Now, time.Minute)
	h.clientset = clientset
	h.controlPlaneNamespace = "controller"
	h.ingressMode = gateway.IngressModeNone
	gw := routedGateway("gateway-id", "gateway")
	phase := "Running"
	gw.Phase = &phase
	var updates []*pb.UpdateGatewayRequest
	client := &fakeGatewayClient{updateFn: func(_ context.Context, request *pb.UpdateGatewayRequest, _ ...grpc.CallOption) (*pb.UpdateGatewayResponse, error) {
		updates = append(updates, request)
		return &pb.UpdateGatewayResponse{}, nil
	}}
	h.reconcileGatewayHealth(context.Background(), client, gw)
	if len(updates) != 1 || updates[0].GetPhase() != "Degraded" || updates[0].GetStatus() != "sandbox image pull access is not ready" {
		t.Fatalf("ready pod bypassed failed image pull access: %v", updates)
	}
	// Removing the operator grant also removes the access requirement.
	t.Setenv("GATEWAY_SANDBOX_IMAGE_PULL_ROLES", "[]")
	phase = "Degraded"
	h.reconcileGatewayHealth(context.Background(), client, gw)
	if len(updates) != 2 || updates[1].GetPhase() != "Running" || updates[1].GetStatus() != "Healthy" {
		t.Fatalf("gateway did not recover after access configuration repair: %v", updates)
	}
}
