package gateway

import (
	"context"
	"errors"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

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
