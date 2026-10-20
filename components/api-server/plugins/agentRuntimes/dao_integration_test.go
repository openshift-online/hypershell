package agentRuntimes_test

import (
	"context"
	"testing"

	"github.com/openshift-online/hypershell/components/api-server/plugins/agentRuntimes"
	"github.com/openshift-online/hypershell/components/api-server/test"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
)

// TestDaoUpdateStatus_DoesNotClobberConcurrentSpecPatch exercises the lost-update
// race the column-scoped UpdateStatus closes. It reproduces the window the control
// plane's status write-back sits in: the reconciler decides to write a status while
// a user PATCH concurrently changes a spec field. With the previous full-row Save,
// the status write-back re-persisted the stale spec snapshot and silently reverted
// the PATCH. The scoped UPDATE must leave the PATCH intact while still applying the
// new status.
func TestDaoUpdateStatus_DoesNotClobberConcurrentSpecPatch(t *testing.T) {
	test.RegisterIntegration(t)
	ctx := context.Background()

	dao := agentRuntimes.NewAgentRuntimeDao(&environments.Environment().Database.SessionFactory)

	created, err := dao.Create(ctx, &agentRuntimes.AgentRuntime{
		Name:      "orig-name",
		ClusterId: "c-1",
		GatewayId: "g-1",
	})
	if err != nil {
		t.Fatalf("create agent runtime: %v", err)
	}

	// A user PATCH lands in the window after the control plane read the row and
	// before its status write-back: it changes a spec column (name).
	toPatch, err := dao.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get for patch: %v", err)
	}
	toPatch.Name = "patched-name"
	if _, err := dao.Replace(ctx, toPatch); err != nil {
		t.Fatalf("spec patch replace: %v", err)
	}

	// The status write-back now lands. It must touch only the status columns.
	msg := "coordinator up"
	updated, err := dao.UpdateStatus(ctx, created.ID, "Running", &msg)
	if err != nil {
		t.Fatalf("update status: %v", err)
	}
	if updated.Status == nil || *updated.Status != "Running" {
		t.Fatalf("status = %v, want Running", updated.Status)
	}
	if updated.StatusMessage == nil || *updated.StatusMessage != msg {
		t.Fatalf("status_message = %v, want %q", updated.StatusMessage, msg)
	}

	// The concurrent spec PATCH must have survived the status write-back.
	final, err := dao.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get final: %v", err)
	}
	if final.Name != "patched-name" {
		t.Fatalf("name = %q, want patched-name (status write-back clobbered the spec PATCH)", final.Name)
	}
	if final.Status == nil || *final.Status != "Running" {
		t.Fatalf("persisted status = %v, want Running", final.Status)
	}
}

// TestDaoUpdateStatus_NotFound verifies a status write to a missing row surfaces an
// error (rather than silently affecting zero rows), so the gRPC path can map it to
// NotFound.
func TestDaoUpdateStatus_NotFound(t *testing.T) {
	test.RegisterIntegration(t)
	ctx := context.Background()

	dao := agentRuntimes.NewAgentRuntimeDao(&environments.Environment().Database.SessionFactory)
	if _, err := dao.UpdateStatus(ctx, "does-not-exist", "Running", nil); err == nil {
		t.Fatalf("expected an error updating a missing agent runtime")
	}
}
