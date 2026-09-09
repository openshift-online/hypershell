package gateway

import (
	"context"
	"errors"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubetesting "k8s.io/client-go/testing"
)

func TestDeleteResourceRequiresFinalizerCompletion(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterrolebindings"}
	object := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "ClusterRoleBinding", "metadata": map[string]interface{}{"name": "gateway", "finalizers": []interface{}{"test/finalizer"}}}}
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), object)
	blocked := true
	client.PrependReactor("delete", "clusterrolebindings", func(kubetesting.Action) (bool, runtime.Object, error) { return blocked, nil, nil })
	if err := deleteResourceAndVerify(context.Background(), client.Resource(gvr), "gateway"); err == nil {
		t.Fatal("accepted deletion was reported as complete")
	}
	blocked = false
	if err := deleteResourceAndVerify(context.Background(), client.Resource(gvr), "gateway"); err != nil {
		t.Fatal(err)
	}
	if err := deleteResourceAndVerify(context.Background(), client.Resource(gvr), "gateway"); err != nil {
		t.Fatalf("repeated deletion failed: %v", err)
	}
}

type deletionKeycloak struct {
	KeycloakClientAPI
	serviceAccountErr error
	parentDeletes     int
	clientIDs         []string
}

func (k *deletionKeycloak) DeleteGatewayServiceAccountClients(context.Context, string) error {
	return k.serviceAccountErr
}
func (k *deletionKeycloak) DeleteConsoleClient(context.Context, string) error {
	k.parentDeletes++
	return nil
}
func (k *deletionKeycloak) DeleteGatewayClient(_ context.Context, id string) error {
	k.clientIDs = append(k.clientIDs, id)
	k.parentDeletes++
	return nil
}

func TestDeleteGatewayRetainsIdentityUntilServiceAccountsAreRemoved(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	identity := &deletionKeycloak{serviceAccountErr: errors.New("identity unavailable")}
	opts := ReconcileOpts{KeycloakClient: identity, GatewayID: "gateway-id", GatewayName: "renamed", GatewayClientID: "original-gateway-id", DatabaseProvider: "deployment", DeploymentDBNamespace: "database"}
	if err := DeleteGatewayResources(context.Background(), client, nil, "gateway-ns", opts); err == nil {
		t.Fatal("service account deletion failure was lost")
	}
	if identity.parentDeletes != 0 {
		t.Fatal("gateway identity deleted before account cleanup")
	}
	identity.serviceAccountErr = nil
	if err := DeleteGatewayResources(context.Background(), client, nil, "gateway-ns", opts); err != nil {
		t.Fatal(err)
	}
	if len(identity.clientIDs) != 1 || identity.clientIDs[0] != "original-gateway-id" {
		t.Fatal("deletion used the mutable gateway name")
	}
	if identity.parentDeletes != 2 {
		t.Fatal("identity cleanup was not retried")
	}
}

func TestCredentialRBACDeletePropagatesFailure(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	client.PrependReactor("delete", "roles", func(kubetesting.Action) (bool, runtime.Object, error) { return true, nil, errors.New("forbidden") })
	if err := deleteCredentialSecretsRBAC(context.Background(), client, "credentials"); err == nil {
		t.Fatal("credential role cleanup failure was lost")
	}
	// The other cleanup action must still be attempted.
	found := false
	for _, action := range client.Actions() {
		if action.GetVerb() == "delete" && action.GetResource().Resource == "rolebindings" {
			found = true
		}
	}
	if !found {
		t.Fatal("partial failure skipped independent cleanup")
	}
}

func TestCredentialDriverReconcileRetriesUnusedRBACCleanup(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	blocked := true
	client.PrependReactor("delete", "roles", func(kubetesting.Action) (bool, runtime.Object, error) {
		if blocked {
			return true, nil, errors.New("forbidden")
		}
		return false, nil, nil
	})
	config := NamespaceConfig{Name: "credentials", Gateway: GatewayConfig{CredentialDriver: &CredentialDriverConfig{Type: "vault"}}}
	if err := reconcileCredentialDriverResources(context.Background(), client, nil, config); err == nil {
		t.Fatal("credential driver change ignored stale secret access")
	}
	blocked = false
	if err := reconcileCredentialDriverResources(context.Background(), client, nil, config); err != nil {
		t.Fatalf("credential driver cleanup did not recover: %v", err)
	}
}
