package gateways_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/plugins/gateways"
	"github.com/openshift-online/hypershell/components/api-server/test"
)

func TestGatewayDeletionStatusCallerScopeAndCompletion(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	client := referenceClient(t, h)
	ctx := referenceContext(t, h, "deletion-owner")
	foreign := referenceContext(t, h, "other-deletion-owner")
	reference := "control/deletion/generation"
	gateway, _, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(openapi.GatewayCreateRequest{Name: "deletion", ExternalReference: &reference}).Execute()
	if err != nil {
		t.Fatal(err)
	}
	service := gateways.Service(&h.Env().Services)
	get := func(want string) *openapi.GatewayDeletionStatus {
		t.Helper()
		result, _, err := client.DefaultAPI.GetGatewayDeletionStatus(ctx).ExternalReference(reference).Execute()
		if err != nil || result.State != want || result.GatewayId != gateway.GetId() {
			t.Fatalf("status = %#v, %v; want %s", result, err, want)
		}
		return result
	}
	active := get("active")
	if active.DeletionRequestedAt != nil || active.DeletionCompletedAt != nil {
		t.Fatal("active gateway has deletion timestamps")
	}
	if err := service.CompleteDeletion(context.Background(), gateway.GetId()); err == nil || err.HttpCode != http.StatusConflict {
		t.Fatalf("active completion = %v", err)
	}
	_, response, err := client.DefaultAPI.GetGatewayDeletionStatus(foreign).ExternalReference(reference).Execute()
	if err == nil || response.StatusCode != http.StatusNotFound {
		t.Fatalf("foreign status = %v, %v", response, err)
	}
	session := h.Env().Database.SessionFactory.New(context.Background())
	if err := session.Exec("UPDATE role_bindings SET deleted_at = now() WHERE gateway_id = ?", gateway.GetId()).Error; err != nil {
		t.Fatal(err)
	}
	if err := session.Exec("UPDATE gateways SET deleted_at = now() WHERE id = ?", gateway.GetId()).Error; err != nil {
		t.Fatal(err)
	}
	requested := get("requested")
	if requested.DeletionRequestedAt == nil || requested.DeletionCompletedAt != nil {
		t.Fatal("invalid requested timestamps")
	}
	if err := service.CompleteDeletion(context.Background(), gateway.GetId()); err != nil {
		t.Fatal(err)
	}
	completed := get("completed")
	if completed.DeletionCompletedAt == nil {
		t.Fatal("completion timestamp is absent")
	}
	if err := service.CompleteDeletion(context.Background(), gateway.GetId()); err != nil {
		t.Fatal(err)
	}
	if !get("completed").DeletionCompletedAt.Equal(*completed.DeletionCompletedAt) {
		t.Fatal("retry changed completion timestamp")
	}
	var bindings int64
	if err := session.Table("role_bindings").Where("gateway_id = ? AND deleted_at IS NULL", gateway.GetId()).Count(&bindings).Error; err != nil {
		t.Fatal(err)
	}
	if bindings != 0 {
		t.Fatal("status restored gateway access")
	}
	_, response, err = client.DefaultAPI.GetGatewayDeletionStatus(foreign).ExternalReference(reference).Execute()
	if err == nil || response.StatusCode != http.StatusNotFound {
		t.Fatalf("foreign tombstone status = %v, %v", response, err)
	}
}

func TestGatewayPendingDeletionsKeysetSurvivesCompletion(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	client := referenceClient(t, h)
	ctx := referenceContext(t, h, "replay-owner")
	service := gateways.Service(&h.Env().Services)
	session := h.Env().Database.SessionFactory.New(context.Background())
	for _, ref := range []string{"control/replay/a", "control/replay/b", "control/replay/c"} {
		gateway, _, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(openapi.GatewayCreateRequest{Name: "replay", ExternalReference: &ref}).Execute()
		if err != nil {
			t.Fatal(err)
		}
		if err := session.Exec("UPDATE gateways SET deleted_at = now() WHERE id = ?", gateway.GetId()).Error; err != nil {
			t.Fatal(err)
		}
	}
	first, err := service.PendingDeletions(context.Background(), "", "", 1)
	if err != nil || len(first) != 1 {
		t.Fatalf("first page = %v, %v", first, err)
	}
	if err := service.CompleteDeletion(context.Background(), first[0].ID); err != nil {
		t.Fatal(err)
	}
	next, err := service.PendingDeletions(context.Background(), first[0].ID, "", 10)
	if err != nil || len(next) != 2 {
		t.Fatalf("completion skipped pending rows: %v, %v", next, err)
	}
	again, err := service.PendingDeletions(context.Background(), "", "", 10)
	if err != nil || len(again) != 2 {
		t.Fatalf("completed row replayed: %v, %v", again, err)
	}
	foreign, err := service.PendingDeletions(context.Background(), "", "other-cluster", 10)
	if err != nil || len(foreign) != 0 {
		t.Fatalf("cross-cluster tombstones: %v, %v", foreign, err)
	}
}
