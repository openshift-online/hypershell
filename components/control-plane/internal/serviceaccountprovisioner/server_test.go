package serviceaccountprovisioner

import (
	"context"
	"errors"
	"strings"
	"testing"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/provisioner/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/serviceaccountkeycloak"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestProvisionReturnsOneTimeCredentialAndMapsDesiredState(t *testing.T) {
	provider := &fakeProvider{configured: true, secret: "one-time-secret"}
	server := NewServer(provider)
	response, err := server.Provision(t.Context(), &pb.ProvisionRequest{Spec: &pb.ServiceAccountSpec{
		ClientId: "client-id", DisplayName: "build bot", GatewayClientId: "gateway-client",
		GatewayId: "gateway-id", ServiceAccountId: "account-id", CreatorUserId: "user-id",
		Role: "openshell-user", ExpectedIssuer: "https://issuer.example/realms/hypershell",
		AccessTokenLifetimeSeconds: 300,
	}})
	if err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	if response.GetClientSecret() != "one-time-secret" || response.GetClientUuid() != "client-uuid" || response.GetSubject() != "subject-id" {
		t.Fatalf("Provision() response = %#v", response)
	}
	if provider.lastSpec.ClientID != "client-id" || provider.lastSpec.GatewayID != "gateway-id" ||
		provider.lastSpec.Role != "openshell-user" || provider.lastSpec.AccessTokenLifetimeSeconds != 300 {
		t.Fatalf("provider spec = %#v", provider.lastSpec)
	}
}

func TestProvisionFailsClosedWithoutKeycloakConfiguration(t *testing.T) {
	_, err := NewServer(&fakeProvider{}).Provision(t.Context(), &pb.ProvisionRequest{Spec: &pb.ServiceAccountSpec{}})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("Provision() code = %s, want Unavailable", status.Code(err))
	}
}

func TestProviderErrorsDoNotExposeProviderDetails(t *testing.T) {
	provider := &fakeProvider{configured: true, err: errors.New("provider body contained super-secret")}
	_, err := NewServer(provider).Disable(t.Context(), &pb.DisableRequest{ClientUuid: "client-uuid"})
	if status.Code(err) != codes.Internal {
		t.Fatalf("Disable() code = %s, want Internal", status.Code(err))
	}
	if strings.Contains(err.Error(), "super-secret") {
		t.Fatalf("Disable() exposed provider details: %v", err)
	}
}

func TestProviderNotFoundUsesStableGRPCStatus(t *testing.T) {
	provider := &fakeProvider{configured: true, err: serviceaccountkeycloak.ErrNotFound}
	_, err := NewServer(provider).Reconcile(t.Context(), &pb.ReconcileRequest{
		Spec: &pb.ServiceAccountSpec{}, ClientUuid: "client-uuid",
	})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("Reconcile() code = %s, want NotFound", status.Code(err))
	}
}

func TestDisableForwardsOwnershipMetadataToProvider(t *testing.T) {
	provider := &fakeProvider{configured: true}
	if _, err := NewServer(provider).Disable(t.Context(), &pb.DisableRequest{
		ClientUuid: "client-uuid", GatewayId: "gateway-id", ServiceAccountId: "account-id",
	}); err != nil {
		t.Fatalf("Disable() error = %v", err)
	}
	if provider.lastUUID != "client-uuid" || provider.lastGatewayID != "gateway-id" || provider.lastServiceAccountID != "account-id" {
		t.Fatalf("Disable() forwarded ownership = %q/%q/%q", provider.lastUUID, provider.lastGatewayID, provider.lastServiceAccountID)
	}
}

func TestDeleteForwardsOwnershipMetadataToProvider(t *testing.T) {
	provider := &fakeProvider{configured: true}
	if _, err := NewServer(provider).Delete(t.Context(), &pb.DeleteRequest{
		ClientUuid: "client-uuid", GatewayId: "gateway-id", ServiceAccountId: "account-id",
	}); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if provider.lastUUID != "client-uuid" || provider.lastGatewayID != "gateway-id" || provider.lastServiceAccountID != "account-id" {
		t.Fatalf("Delete() forwarded ownership = %q/%q/%q", provider.lastUUID, provider.lastGatewayID, provider.lastServiceAccountID)
	}
}

type fakeProvider struct {
	configured           bool
	secret               string
	err                  error
	lastSpec             serviceaccountkeycloak.ServiceAccountSpec
	lastUUID             string
	lastGatewayID        string
	lastServiceAccountID string
}

func (f *fakeProvider) Configured() bool { return f.configured }

func (f *fakeProvider) ProvisionServiceAccount(_ context.Context, spec serviceaccountkeycloak.ServiceAccountSpec) (*serviceaccountkeycloak.ProvisionedServiceAccount, error) {
	f.lastSpec = spec
	if f.err != nil {
		return nil, f.err
	}
	return &serviceaccountkeycloak.ProvisionedServiceAccount{
		ClientUUID: "client-uuid", ClientID: spec.ClientID, ClientSecret: f.secret, Subject: "subject-id",
	}, nil
}

func (f *fakeProvider) ReconcileServiceAccount(_ context.Context, spec serviceaccountkeycloak.ServiceAccountSpec, _, _ string, _ bool) error {
	f.lastSpec = spec
	return f.err
}

func (f *fakeProvider) DisableServiceAccount(_ context.Context, uuid, gatewayID, serviceAccountID string) error {
	f.lastUUID, f.lastGatewayID, f.lastServiceAccountID = uuid, gatewayID, serviceAccountID
	return f.err
}

func (f *fakeProvider) DeleteServiceAccount(_ context.Context, uuid, gatewayID, serviceAccountID string) error {
	f.lastUUID, f.lastGatewayID, f.lastServiceAccountID = uuid, gatewayID, serviceAccountID
	return f.err
}
func (f *fakeProvider) DeleteManagedServiceAccount(context.Context, string, string) error {
	return f.err
}
func (f *fakeProvider) DeleteGatewayServiceAccounts(context.Context, string) error { return f.err }
func (f *fakeProvider) ListManagedClients(context.Context, string) ([]serviceaccountkeycloak.ManagedClient, error) {
	return nil, f.err
}
