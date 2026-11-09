package reconciler

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
	"google.golang.org/grpc"
)

const generatedConfigGatewayID = "3LrslB1mTv5s6yE6URRnajMlu7e"

func generatedConfigKeycloak() *gateway.KeycloakConfig {
	return &gateway.KeycloakConfig{
		ServerURL: "https://keycloak.example.test/",
		Realm:     "hypershell",
	}
}

// recordingGatewayClient fakes the API server's UpdateGateway for the OIDC
// write: it records each request and answers with the scripted stored record.
type recordingGatewayClient struct {
	pb.GatewayServiceClient
	requests []*pb.UpdateGatewayRequest
	respond  func(in *pb.UpdateGatewayRequest) (*pb.UpdateGatewayResponse, error)
}

func (f *recordingGatewayClient) UpdateGateway(_ context.Context, in *pb.UpdateGatewayRequest, _ ...grpc.CallOption) (*pb.UpdateGatewayResponse, error) {
	f.requests = append(f.requests, in)
	return f.respond(in)
}

func desiredOIDCJSON(t *testing.T, clientID string) string {
	t.Helper()
	raw, err := json.Marshal(gateway.DesiredOIDCConfig(generatedConfigKeycloak(), clientID))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func newGeneratedConfigReconciler(client pb.GatewayServiceClient) *GatewayReconciler {
	return &GatewayReconciler{
		active:                map[string]struct{}{},
		keycloakConfig:        generatedConfigKeycloak(),
		gatewayClientOverride: client,
	}
}

// Verifies: PLAT-DATA-06.06
func TestPersistGeneratedOIDC_NewGatewayAppliesPersistedGeneration(t *testing.T) {
	t.Setenv("GATEWAY_OIDC_ISSUER_URL", "")
	event := &pb.Gateway{
		Metadata:   &pb.ObjectReference{Id: generatedConfigGatewayID},
		Name:       "new-gateway",
		Generation: 1,
	}
	client := &recordingGatewayClient{respond: func(in *pb.UpdateGatewayRequest) (*pb.UpdateGatewayResponse, error) {
		// The API server treats the OIDC write as a desired-state change.
		return &pb.UpdateGatewayResponse{Gateway: &pb.Gateway{
			Metadata:   &pb.ObjectReference{Id: generatedConfigGatewayID},
			Name:       "new-gateway",
			Generation: 2,
			Oidc:       in.Oidc,
		}}, nil
	}}
	r := newGeneratedConfigReconciler(client)

	snapshot, err := r.persistGeneratedOIDC(t.Context(), generatedConfigGatewayID, event, generatedConfigGatewayID)
	if err != nil {
		t.Fatalf("persistGeneratedOIDC: %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("UpdateGateway calls = %d, want 1", len(client.requests))
	}
	req := client.requests[0]
	if req.GetObservedGeneration() != 0 || req.Phase != nil || req.Status != nil {
		t.Errorf("OIDC write must carry only oidc, got %+v", req)
	}
	if got, want := req.GetOidc(), desiredOIDCJSON(t, generatedConfigGatewayID); got != want {
		t.Errorf("persisted oidc = %s, want %s", got, want)
	}
	// The pass applies and acknowledges the generation that contains its own
	// generated configuration, not the generation of the triggering event.
	if snapshot.GetGeneration() != 2 {
		t.Errorf("snapshot generation = %d, want 2 (the generation that includes the persisted OIDC)", snapshot.GetGeneration())
	}
	var applied gateway.OIDCConfig
	if err := json.Unmarshal([]byte(snapshot.GetOidc()), &applied); err != nil {
		t.Fatalf("snapshot oidc: %v", err)
	}
	if applied.Issuer != "https://keycloak.example.test/realms/hypershell" || applied.ClientID != generatedConfigGatewayID || applied.Audience != generatedConfigGatewayID {
		t.Errorf("snapshot oidc = %+v, want the generated issuer and client identity", applied)
	}
}

// Verifies: PLAT-DATA-06.06
func TestPersistGeneratedOIDC_StoredConfigurationIsNotRewritten(t *testing.T) {
	t.Setenv("GATEWAY_OIDC_ISSUER_URL", "")
	// A later full pass (for example a destination-CA update) of a gateway whose
	// generated configuration is already stored must not write it again: an extra
	// write would be a no-op at best, and the pass keeps the event's generation.
	stored := desiredOIDCJSON(t, generatedConfigGatewayID)
	event := &pb.Gateway{
		Metadata:   &pb.ObjectReference{Id: generatedConfigGatewayID},
		Name:       "converged",
		Generation: 2,
		Oidc:       &stored,
	}
	client := &recordingGatewayClient{respond: func(*pb.UpdateGatewayRequest) (*pb.UpdateGatewayResponse, error) {
		t.Fatal("UpdateGateway must not be called when the stored OIDC configuration matches")
		return nil, nil
	}}
	r := newGeneratedConfigReconciler(client)

	snapshot, err := r.persistGeneratedOIDC(t.Context(), generatedConfigGatewayID, event, generatedConfigGatewayID)
	if err != nil {
		t.Fatalf("persistGeneratedOIDC: %v", err)
	}
	if snapshot != event {
		t.Errorf("snapshot = %+v, want the event payload itself", snapshot)
	}
}

// Verifies: PLAT-DATA-06.06
func TestPersistGeneratedOIDC_RenamedGatewayKeepsStoredClientIdentity(t *testing.T) {
	t.Setenv("GATEWAY_OIDC_ISSUER_URL", "")
	legacyClientID := "original-name-" + generatedConfigGatewayID
	stored := desiredOIDCJSON(t, legacyClientID)
	event := &pb.Gateway{
		Metadata:   &pb.ObjectReference{Id: generatedConfigGatewayID},
		Name:       "renamed",
		Generation: 3,
		Oidc:       &stored,
	}
	clientID, err := gatewayKeycloakClientID(generatedConfigGatewayID, event)
	if err != nil {
		t.Fatalf("gatewayKeycloakClientID: %v", err)
	}
	client := &recordingGatewayClient{respond: func(*pb.UpdateGatewayRequest) (*pb.UpdateGatewayResponse, error) {
		t.Fatal("a rename must not rewrite the persisted OIDC client identity")
		return nil, nil
	}}
	r := newGeneratedConfigReconciler(client)

	if _, err := r.persistGeneratedOIDC(t.Context(), generatedConfigGatewayID, event, clientID); err != nil {
		t.Fatalf("persistGeneratedOIDC: %v", err)
	}
}

// Verifies: PLAT-DATA-06.07
func TestPersistGeneratedOIDC_SnapshotCarriesStoredStateNotStaleEvent(t *testing.T) {
	t.Setenv("GATEWAY_OIDC_ISSUER_URL", "")
	// The event is stale: while it was queued another control-plane write moved
	// the record to generation 3 with a new image. The OIDC write lands on top
	// (generation 4). The pass must build from the stored record, so the
	// generation it acknowledges is the one whose configuration it applied.
	oldImage := "quay.io/example/gateway:old"
	newImage := "quay.io/example/gateway:new"
	event := &pb.Gateway{
		Metadata:   &pb.ObjectReference{Id: generatedConfigGatewayID},
		Name:       "stale-event",
		Generation: 2,
		Image:      &oldImage,
	}
	client := &recordingGatewayClient{respond: func(in *pb.UpdateGatewayRequest) (*pb.UpdateGatewayResponse, error) {
		return &pb.UpdateGatewayResponse{Gateway: &pb.Gateway{
			Metadata:   &pb.ObjectReference{Id: generatedConfigGatewayID},
			Name:       "stale-event",
			Generation: 4,
			Image:      &newImage,
			Oidc:       in.Oidc,
		}}, nil
	}}
	r := newGeneratedConfigReconciler(client)

	snapshot, err := r.persistGeneratedOIDC(t.Context(), generatedConfigGatewayID, event, generatedConfigGatewayID)
	if err != nil {
		t.Fatalf("persistGeneratedOIDC: %v", err)
	}
	if snapshot.GetGeneration() != 4 || snapshot.GetImage() != newImage {
		t.Errorf("snapshot = generation %d image %q, want generation 4 image %q (the stored record)", snapshot.GetGeneration(), snapshot.GetImage(), newImage)
	}
}

// Verifies: OSG-OIDC-05.01
func TestPersistGeneratedOIDC_WriteFailureIsReturned(t *testing.T) {
	t.Setenv("GATEWAY_OIDC_ISSUER_URL", "")
	for name, respond := range map[string]func(*pb.UpdateGatewayRequest) (*pb.UpdateGatewayResponse, error){
		"rpc error": func(*pb.UpdateGatewayRequest) (*pb.UpdateGatewayResponse, error) {
			return nil, errors.New("unavailable")
		},
		"empty response": func(*pb.UpdateGatewayRequest) (*pb.UpdateGatewayResponse, error) {
			return &pb.UpdateGatewayResponse{}, nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := newGeneratedConfigReconciler(&recordingGatewayClient{respond: respond})
			event := &pb.Gateway{Metadata: &pb.ObjectReference{Id: generatedConfigGatewayID}, Generation: 1}
			snapshot, err := r.persistGeneratedOIDC(t.Context(), generatedConfigGatewayID, event, generatedConfigGatewayID)
			if err == nil {
				t.Fatalf("persistGeneratedOIDC succeeded with snapshot %+v, want an error", snapshot)
			}
		})
	}
}

// Verifies: OSG-OIDC-05.01
func TestHandle_OIDCWriteFailureStopsBeforeApplyAndKeepsObservedGeneration(t *testing.T) {
	t.Setenv("GATEWAY_OIDC_ISSUER_URL", "")
	client := &recordingGatewayClient{respond: func(*pb.UpdateGatewayRequest) (*pb.UpdateGatewayResponse, error) {
		return nil, errors.New("api server unavailable")
	}}
	r := newGeneratedConfigReconciler(client)
	observed := int64(0)

	err := r.Handle(t.Context(), watcher.Event[*pb.Gateway]{
		Type:       watcher.EventCreated,
		ResourceID: generatedConfigGatewayID,
		Resource: &pb.Gateway{
			Metadata:           &pb.ObjectReference{Id: generatedConfigGatewayID},
			Name:               "new-gateway",
			Generation:         1,
			ObservedGeneration: &observed,
		},
	})
	if err == nil {
		t.Fatal("Handle succeeded, want the OIDC write failure surfaced for retry")
	}
	// The only write is the failed OIDC write: no phase, condition, or
	// observed_generation update, so nothing claims the generation was applied.
	if len(client.requests) != 1 || client.requests[0].Oidc == nil {
		t.Fatalf("UpdateGateway requests = %+v, want exactly the failed OIDC write", client.requests)
	}
}

func TestDesiredOIDCConfig_IssuerOverride(t *testing.T) {
	t.Setenv("GATEWAY_OIDC_ISSUER_URL", "https://keycloak.public.example.test/realms/hypershell")
	got := gateway.DesiredOIDCConfig(generatedConfigKeycloak(), generatedConfigGatewayID)
	if got.Issuer != "https://keycloak.public.example.test/realms/hypershell" {
		t.Errorf("Issuer = %q, want the GATEWAY_OIDC_ISSUER_URL override", got.Issuer)
	}
}
