package serviceAccounts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/plugins/gateways"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
	trexerrors "github.com/openshift-online/rh-trex-ai/pkg/errors"
	"gorm.io/gorm"
)

func TestCreateHandlerReturnsTheCredentialOnceWithCacheProtection(t *testing.T) {
	dao := newMemoryDAO()
	svc := newTestService(dao, &fakeKeycloak{configured: true}, testBindings("creator", "gateway:owner"), time.Now().UTC())
	handler := NewHandler(svc)
	body := bytes.NewBufferString(`{"name":"deploy-bot","role":"openshell-user"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/hypershell/v1/gateways/gateway-id/service_accounts", body)
	request = mux.SetURLVars(request, map[string]string{"gateway_id": "gateway-id"})
	request = request.WithContext(context.WithValue(request.Context(), rbac.ContextUserIDKey, "creator"))
	recorder := httptest.NewRecorder()

	handler.Create(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Cache-Control") != "no-store" || recorder.Header().Get("Pragma") != "no-cache" {
		t.Fatalf("cache headers = %#v", recorder.Header())
	}
	var created map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	credential, ok := created["credential"].(map[string]any)
	if !ok || credential["client_secret"] != "one-time-secret" {
		t.Fatalf("create credential = %#v", created["credential"])
	}
	if credential["gateway_endpoint"] != "https://gateway.example:443" {
		t.Fatalf("gateway endpoint = %q, want HTTPS endpoint", credential["gateway_endpoint"])
	}

	getRequest := httptest.NewRequest(http.MethodGet, "/api/hypershell/v1/gateways/gateway-id/service_accounts/"+created["id"].(string), nil)
	getRequest = mux.SetURLVars(getRequest, map[string]string{
		"gateway_id": "gateway-id", "service_account_id": created["id"].(string),
	})
	getRequest = getRequest.WithContext(context.WithValue(getRequest.Context(), rbac.ContextUserIDKey, "creator"))
	getRecorder := httptest.NewRecorder()
	handler.Get(getRecorder, getRequest)
	if getRecorder.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", getRecorder.Code, getRecorder.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(getRecorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if _, exists := got["credential"]; exists {
		t.Fatalf("get response leaked credential: %#v", got["credential"])
	}
}

func TestGatewayEndpointNormalizesGRPCSchemesForOpenShell(t *testing.T) {
	for _, test := range []struct {
		name string
		in   string
		want string
	}{
		{name: "TLS", in: "grpcs://gateway.example:443", want: "https://gateway.example:443"},
		{name: "plaintext", in: "grpc://gateway.example:80", want: "http://gateway.example:80"},
		{name: "already HTTP", in: "https://gateway.example:443", want: "https://gateway.example:443"},
		{name: "trimmed", in: "  grpcs://gateway.example:443  ", want: "https://gateway.example:443"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := gatewayEndpoint(test.in); got != test.want {
				t.Fatalf("gatewayEndpoint(%q) = %q, want %q", test.in, got, test.want)
			}
		})
	}
}

func TestCreateEnforcesRoleCapAndKeepsCredentialOutOfPersistence(t *testing.T) {
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name       string
		binding    string
		requested  string
		wantStatus int
		wantRole   string
	}{
		{name: "owner may select admin", binding: "gateway:owner", requested: RoleAdmin, wantRole: RoleAdmin},
		{name: "owner may select user", binding: "gateway:owner", requested: RoleUser, wantRole: RoleUser},
		{name: "viewer defaults to user", binding: "gateway:viewer", requested: "", wantRole: RoleUser},
		{name: "viewer cannot select admin", binding: "gateway:viewer", requested: RoleAdmin, wantStatus: 403},
	} {
		t.Run(test.name, func(t *testing.T) {
			dao := newMemoryDAO()
			kc := &fakeKeycloak{configured: true}
			service := newTestService(dao, kc, testBindings("creator", test.binding), now)
			result, problem := service.Create(t.Context(), "gateway-id", "creator", CreateInput{Name: "deploy-bot", Role: test.requested})
			if test.wantStatus != 0 {
				if problem == nil || problem.Status != test.wantStatus {
					t.Fatalf("problem = %#v, want status %d", problem, test.wantStatus)
				}
				if kc.provisionCalls != 0 {
					t.Error("forbidden request reached Keycloak")
				}
				return
			}
			if problem != nil {
				t.Fatalf("Create() problem = %v", problem)
			}
			if result.Account.Role != test.wantRole || kc.lastSpec.Role != test.wantRole {
				t.Errorf("role = %q, Keycloak role = %q, want %q", result.Account.Role, kc.lastSpec.Role, test.wantRole)
			}
			if result.Credential.ClientSecret != kc.secret {
				t.Error("one-time response omitted credential")
			}
			if result.Account.Status != StatusReady || result.Account.Subject != "subject-id" {
				t.Errorf("account = %#v", result.Account)
			}
			if got := result.Account.ExpiresAt.Sub(now); got != DefaultExpiration {
				t.Errorf("default expiration = %s, want %s", got, DefaultExpiration)
			}
			persistedJSON, _ := json.Marshal(dao.items[result.Account.ID])
			if strings.Contains(string(persistedJSON), kc.secret) || strings.Contains(string(persistedJSON), "client_secret\":\""+kc.secret) {
				t.Fatalf("persisted model leaked credential: %s", persistedJSON)
			}
			for _, audit := range dao.audits {
				auditJSON, _ := json.Marshal(audit)
				if strings.Contains(string(auditJSON), kc.secret) {
					t.Fatalf("audit leaked credential: %s", auditJSON)
				}
			}
		})
	}
}

func TestViewerVisibilityIsCreatorOnlyWhileOwnerCanManageAll(t *testing.T) {
	dao := newMemoryDAO()
	now := time.Now().UTC()
	dao.seed(
		&OpenShellGatewayServiceAccount{GatewayID: "gateway-id", Name: "mine", CreatedByUserID: "viewer-a", Status: StatusReady, Role: RoleUser, CredentialType: CredentialTypeClientSecret, KeycloakClientID: "client-a", Subject: "sub-a", ExpiresAt: now.Add(time.Hour)},
		&OpenShellGatewayServiceAccount{GatewayID: "gateway-id", Name: "theirs", CreatedByUserID: "viewer-b", Status: StatusReady, Role: RoleUser, CredentialType: CredentialTypeClientSecret, KeycloakClientID: "client-b", Subject: "sub-b", ExpiresAt: now.Add(time.Hour)},
	)
	bindings := fakeBindings{byUser: map[string][]rbac.BindingSummary{
		"viewer-a": binding("gateway:viewer"), "owner": binding("gateway:owner"),
	}}
	service := newTestService(dao, &fakeKeycloak{configured: true}, bindings, now)

	items, total, access, problem := service.List(t.Context(), "gateway-id", "viewer-a", ListOptions{Page: 1, Size: 20, Sort: "created_at", Order: "asc"})
	if problem != nil || len(items) != 1 || total != 1 || items[0].CreatedByUserID != "viewer-a" || access.CanManageAll {
		t.Fatalf("viewer list = %#v total=%d access=%#v problem=%v", items, total, access, problem)
	}
	if _, _, problem := service.Get(t.Context(), "gateway-id", itemsIDByCreator(dao, "viewer-b"), "viewer-a"); problem == nil || problem.Status != 404 {
		t.Fatalf("viewer cross-account Get() problem = %#v, want 404", problem)
	}
	ownerItems, ownerTotal, ownerAccess, problem := service.List(t.Context(), "gateway-id", "owner", ListOptions{Page: 1, Size: 20, Sort: "created_at", Order: "asc"})
	if problem != nil || len(ownerItems) != 2 || ownerTotal != 2 || !ownerAccess.CanManageAll {
		t.Fatalf("owner list = %#v total=%d access=%#v problem=%v", ownerItems, ownerTotal, ownerAccess, problem)
	}
}

func TestCreateHidesGatewayReadinessWithoutAnExactBinding(t *testing.T) {
	dao := newMemoryDAO()
	bindings := fakeBindings{byUser: map[string][]rbac.BindingSummary{
		"platform-admin": {{RoleName: "platform:admin", Scope: "global"}},
	}}
	svc := newTestService(dao, &fakeKeycloak{configured: true}, bindings, time.Now().UTC())
	internal := svc.(*service)
	gateway := internal.gateways.(fakeGateway).gateway
	phase := "Provisioning"
	gateway.Phase = &phase
	gateway.Oidc = nil

	for _, userID := range []string{"unbound-user", "platform-admin"} {
		if _, problem := svc.Create(t.Context(), gateway.ID, userID, CreateInput{Name: "hidden"}); problem == nil || problem.Status != 404 {
			t.Fatalf("Create() for %s problem = %#v, want 404", userID, problem)
		}
	}
	dao.seed(&OpenShellGatewayServiceAccount{
		GatewayID: gateway.ID, Name: "hidden", CreatedByUserID: "someone-else", Status: StatusReady,
		Role: RoleUser, CredentialType: CredentialTypeClientSecret, KeycloakClientID: "hidden-client",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	})
	if _, _, problem := svc.Get(t.Context(), gateway.ID, itemsIDByCreator(dao, "someone-else"), "unbound-user"); problem == nil || problem.Status != 404 {
		t.Fatalf("Get() problem = %#v, want 404", problem)
	}
}

func TestCreateFailureRemovesAProvenAbsentCredentialReservation(t *testing.T) {
	dao := newMemoryDAO()
	kc := &fakeKeycloak{configured: true, provisionErr: errors.New("provider unavailable")}
	service := newTestService(dao, kc, testBindings("creator", "gateway:owner"), time.Now().UTC())

	if _, problem := service.Create(t.Context(), "gateway-id", "creator", CreateInput{Name: "deploy-bot"}); problem == nil || problem.Status != 503 {
		t.Fatalf("Create() problem = %#v, want 503", problem)
	}
	if len(dao.items) != 0 {
		t.Fatalf("failed reservation was not removed: %#v", dao.items)
	}
	if kc.deleteManagedCalls != 1 {
		t.Fatalf("DeleteManagedServiceAccount() calls = %d, want 1", kc.deleteManagedCalls)
	}
	if len(dao.audits) < 2 || dao.audits[len(dao.audits)-1].Outcome != "failed" {
		t.Fatalf("creation failure audit = %#v", dao.audits)
	}
}

func TestReconcileDowngradesAdminAndRevokesWhenBindingIsRemoved(t *testing.T) {
	dao := newMemoryDAO()
	now := time.Now().UTC()
	account := &OpenShellGatewayServiceAccount{
		GatewayID: "gateway-id", Name: "bot", CreatedByUserID: "creator", Status: StatusReady,
		Role: RoleAdmin, CredentialType: CredentialTypeClientSecret, KeycloakClientID: "client-id",
		KeycloakClientUUID: "client-uuid", Subject: "subject-id", ExpiresAt: now.Add(time.Hour),
	}
	dao.seed(account)
	bindings := testBindings("creator", "gateway:viewer")
	kc := &fakeKeycloak{configured: true}
	service := newTestService(dao, kc, bindings, now)

	if err := service.ReconcileOnce(t.Context()); err != nil {
		t.Fatalf("ReconcileOnce() error = %v", err)
	}
	stored := dao.items[account.ID]
	if stored.Role != RoleUser || kc.lastSpec.Role != RoleUser || stored.Status != StatusReady {
		t.Fatalf("downgraded account = %#v, Keycloak role = %q", stored, kc.lastSpec.Role)
	}

	bindings.byUser["creator"] = nil
	if err := service.ReconcileOnce(t.Context()); err != nil {
		t.Fatalf("ReconcileOnce() after removal error = %v", err)
	}
	stored = dao.items[account.ID]
	if stored.Status != StatusRevoked || stored.RevokedAt == nil || kc.disableCalls == 0 {
		t.Fatalf("revoked account = %#v, disableCalls=%d", stored, kc.disableCalls)
	}
}

func TestReconcileExpiresAndContinuesCheckingTerminalAccounts(t *testing.T) {
	dao := newMemoryDAO()
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	reason := "previous_failure"
	account := &OpenShellGatewayServiceAccount{
		GatewayID: "gateway-id", Name: "bot", CreatedByUserID: "creator", Status: StatusReady,
		Role: RoleUser, CredentialType: CredentialTypeClientSecret, KeycloakClientID: "client-id",
		KeycloakClientUUID: "client-uuid", Subject: "subject-id", ExpiresAt: now, LastError: &reason,
	}
	dao.seed(account)
	kc := &fakeKeycloak{configured: true}
	service := newTestService(dao, kc, testBindings("creator", "gateway:viewer"), now)

	if err := service.ReconcileOnce(t.Context()); err != nil {
		t.Fatalf("ReconcileOnce() error = %v", err)
	}
	stored := dao.items[account.ID]
	if stored.Status != StatusExpired || stored.RevokedAt == nil || stored.LastError != nil || kc.disableCalls != 1 {
		t.Fatalf("expired account = %#v, disableCalls=%d", stored, kc.disableCalls)
	}

	if err := service.ReconcileOnce(t.Context()); err != nil {
		t.Fatalf("ReconcileOnce() terminal drift check error = %v", err)
	}
	if kc.disableCalls != 2 {
		t.Fatalf("terminal drift disable calls = %d, want 2", kc.disableCalls)
	}
}

func TestRevokePersistsForRetryAndDeletePerformsFinalCleanup(t *testing.T) {
	dao := newMemoryDAO()
	now := time.Now().UTC()
	account := &OpenShellGatewayServiceAccount{
		GatewayID: "gateway-id", Name: "bot", CreatedByUserID: "creator", Status: StatusReady,
		Role: RoleUser, CredentialType: CredentialTypeClientSecret, KeycloakClientID: "client-id",
		KeycloakClientUUID: "client-uuid", Subject: "subject-id", ExpiresAt: now.Add(time.Hour),
	}
	dao.seed(account)
	kc := &fakeKeycloak{configured: true, disableErr: errors.New("provider unavailable")}
	service := newTestService(dao, kc, testBindings("creator", "gateway:owner"), now)

	pending, complete, problem := service.Revoke(t.Context(), "gateway-id", account.ID, "creator")
	if problem != nil || complete || pending.Status != StatusRevoking {
		t.Fatalf("Revoke() account=%#v complete=%t problem=%v", pending, complete, problem)
	}
	if stored := dao.items[account.ID]; stored.Status != StatusRevoking || stored.LastError == nil {
		t.Fatalf("persisted revocation = %#v", stored)
	}

	kc.disableErr = nil
	if err := service.ReconcileOnce(t.Context()); err != nil {
		t.Fatalf("ReconcileOnce() retry error = %v", err)
	}
	if stored := dao.items[account.ID]; stored.Status != StatusRevoked || stored.RevokedAt == nil {
		t.Fatalf("retried revocation = %#v", stored)
	}

	_, deleted, problem := service.Delete(t.Context(), "gateway-id", account.ID, "creator")
	if problem != nil || !deleted {
		t.Fatalf("Delete() complete=%t problem=%v", deleted, problem)
	}
	if _, exists := dao.items[account.ID]; exists {
		t.Fatal("deleted account remains visible")
	}
	if len(kc.deletedUUIDs) != 1 || kc.deletedUUIDs[0] != "client-uuid" {
		t.Fatalf("deleted Keycloak UUIDs = %v", kc.deletedUUIDs)
	}
	// The UUID-targeted deletion must carry the exact gateway and service-account
	// ownership so the control plane can refuse to disable or delete a client that
	// does not belong to this account.
	want := ownershipArgs{UUID: "client-uuid", GatewayID: "gateway-id", ServiceAccountID: account.ID}
	if kc.lastOwnership != want {
		t.Fatalf("delete ownership = %#v, want %#v", kc.lastOwnership, want)
	}
}

func TestCleanupGatewayDeletesIdentitiesBeforeRecords(t *testing.T) {
	dao := newMemoryDAO()
	now := time.Now().UTC()
	dao.seed(
		&OpenShellGatewayServiceAccount{GatewayID: "gateway-id", Name: "first", CreatedByUserID: "creator", Status: StatusReady, Role: RoleUser, CredentialType: CredentialTypeClientSecret, KeycloakClientID: "client-1", ExpiresAt: now.Add(time.Hour)},
		&OpenShellGatewayServiceAccount{GatewayID: "gateway-id", Name: "second", CreatedByUserID: "creator", Status: StatusRevoked, Role: RoleUser, CredentialType: CredentialTypeClientSecret, KeycloakClientID: "client-2", ExpiresAt: now.Add(time.Hour)},
	)
	kc := &fakeKeycloak{configured: true}
	service := newTestService(dao, kc, testBindings("creator", "gateway:owner"), now)

	if err := service.CleanupGateway(t.Context(), "gateway-id", nil); err != nil {
		t.Fatalf("CleanupGateway() error = %v", err)
	}
	if kc.deleteGatewayCalls != 1 {
		t.Fatalf("DeleteGatewayServiceAccounts() calls = %d, want 1", kc.deleteGatewayCalls)
	}
	if len(dao.items) != 0 {
		t.Fatalf("gateway cleanup retained records: %#v", dao.items)
	}
	if len(dao.audits) != 2 {
		t.Fatalf("gateway cleanup audits = %d, want 2", len(dao.audits))
	}
}

// TestCleanupGatewayHoldsBarrierThroughGatewayDeletion proves the fix for the
// teardown race: a Create that races a gateway deletion must block on the
// per-gateway lifecycle lock until CleanupGateway has deleted the gateway row
// (via finalize) under that same lock, and must then observe the gateway as gone
// and provision nothing. Before the fix the row was deleted only after the lock
// was released, so a blocked Create could wake, see the still-present gateway,
// and mint an orphaned live credential.
func TestCleanupGatewayHoldsBarrierThroughGatewayDeletion(t *testing.T) {
	dao := newMemoryDAO()
	now := time.Now().UTC()
	kc := &fakeKeycloak{configured: true}
	locks := newBlockingLockFactory()
	gw := &mutableGateway{gateway: testGateway()}
	svc := newTestServiceWith(dao, kc, testBindings("creator", "gateway:owner"), now, gw, locks)

	createStarted := make(chan struct{})
	createDone := make(chan *APIError, 1)

	// finalize runs under CleanupGateway's held lock and stands in for the
	// gateway-row deletion. While it runs, a concurrent Create must be unable to
	// complete because it is blocked acquiring the same lock.
	finalize := func(context.Context) error {
		go func() {
			close(createStarted)
			_, problem := svc.Create(context.Background(), "gateway-id", "creator", CreateInput{Name: "bot"})
			createDone <- problem
		}()
		<-createStarted
		select {
		case <-createDone:
			t.Error("Create completed while CleanupGateway still held the lock")
		case <-time.After(50 * time.Millisecond):
		}
		gw.delete()
		return nil
	}

	if err := svc.CleanupGateway(context.Background(), "gateway-id", finalize); err != nil {
		t.Fatalf("CleanupGateway() error = %v", err)
	}

	problem := <-createDone
	if problem == nil {
		t.Fatal("Create succeeded after gateway teardown; expected rejection")
	}
	if kc.provisionCalls != 0 {
		t.Fatalf("Create provisioned %d credential(s) for a deleted gateway; want 0", kc.provisionCalls)
	}
}

// TestReconcileDrainsDueWorkConcurrentlyUnderLatency proves that a backlog of
// due expirations spread across many gateways is disabled concurrently and fully
// drained in a single pass, so a slow per-account Keycloak round-trip cannot push
// disablement past the one-minute bound by serializing the whole backlog.
func TestReconcileDrainsDueWorkConcurrentlyUnderLatency(t *testing.T) {
	dao := newMemoryDAO()
	now := time.Now().UTC()
	const total = 16
	for i := 0; i < total; i++ {
		account := &OpenShellGatewayServiceAccount{
			GatewayID:          fmt.Sprintf("gateway-%02d", i),
			Name:               fmt.Sprintf("bot-%02d", i),
			CreatedByUserID:    "creator",
			Status:             StatusReady,
			Role:               RoleUser,
			CredentialType:     CredentialTypeClientSecret,
			KeycloakClientUUID: fmt.Sprintf("uuid-%02d", i),
			ExpiresAt:          now.Add(-time.Hour),
		}
		account.ID = fmt.Sprintf("acct-%02d", i)
		dao.seed(account)
	}
	kc := &latencyKeycloak{delay: 20 * time.Millisecond}
	svc := newTestServiceWith(dao, kc, testBindings("creator", "gateway:owner"), now, fakeGateway{gateway: testGateway()}, db.NewNoOpLockFactory())

	if err := svc.ReconcileOnce(t.Context()); err != nil {
		t.Fatalf("ReconcileOnce() error = %v", err)
	}
	if got := kc.disableCalls.Load(); got != total {
		t.Fatalf("disable calls = %d, want %d; due backlog not fully drained", got, total)
	}
	if got := kc.maxInFlight.Load(); got < 2 {
		t.Fatalf("max concurrent disables = %d; due work did not run concurrently", got)
	}
	if got := kc.maxInFlight.Load(); got > reconcileDueConcurrency {
		t.Fatalf("max concurrent disables = %d, exceeds bound %d", got, reconcileDueConcurrency)
	}
	for id, account := range dao.items {
		if account.Status != StatusExpired {
			t.Fatalf("account %s status = %q, want expired", id, account.Status)
		}
	}
}

// TestReconcileDrainsDueWorkPastARetainedFirstPage proves the due drain advances
// through every scan page even when the first page stays fully eligible. The
// earliest-sorting page is a set of freshly provisioning rows that reconcile to a
// no-op and therefore remain in the due/transitional set every scan. A drain that
// re-queried the same ordered first page (skipping already-attempted rows) would
// see an all-attempted page, stop, and never process the later due rows. Keyset
// pagination over the immutable (expires_at, id) advances past the retained page,
// so the later expirations are still disabled within the cycle.
func TestReconcileDrainsDueWorkPastARetainedFirstPage(t *testing.T) {
	dao := newMemoryDAO()
	now := time.Now().UTC()

	const stickyPageSize = 3
	// First page: provisioning rows with the earliest expiry so they sort first.
	// A recent updated_at keeps them inside the reclaim deadline, so each one
	// reconciles to a no-op and stays Provisioning (still eligible) every scan.
	for i := 0; i < stickyPageSize; i++ {
		sticky := &OpenShellGatewayServiceAccount{
			GatewayID: fmt.Sprintf("sticky-gateway-%02d", i), Name: fmt.Sprintf("sticky-%02d", i),
			CreatedByUserID: "creator", Status: StatusProvisioning, Role: RoleUser,
			CredentialType: CredentialTypeClientSecret,
			ExpiresAt:      now.Add(-3 * time.Hour).Add(time.Duration(i) * time.Minute),
		}
		sticky.ID = fmt.Sprintf("sticky-%02d", i)
		sticky.CreatedAt = now
		sticky.UpdatedAt = now
		dao.seed(sticky)
	}

	// Later pages: due Ready rows that sort after the retained first page and must
	// still be disabled. Four rows over a three-row page force multiple later pages.
	const dueTotal = 4
	for i := 0; i < dueTotal; i++ {
		due := &OpenShellGatewayServiceAccount{
			GatewayID: fmt.Sprintf("due-gateway-%02d", i), Name: fmt.Sprintf("due-%02d", i),
			CreatedByUserID: "creator", Status: StatusReady, Role: RoleUser,
			CredentialType: CredentialTypeClientSecret, KeycloakClientUUID: fmt.Sprintf("due-uuid-%02d", i),
			ExpiresAt: now.Add(-time.Hour).Add(time.Duration(i) * time.Minute),
		}
		due.ID = fmt.Sprintf("due-%02d", i)
		dao.seed(due)
	}

	// latencyKeycloak counts disables atomically, so the concurrent drain workers
	// are race-safe under -race.
	kc := &latencyKeycloak{}
	svc := newTestServiceWith(dao, kc, testBindings("creator", "gateway:owner"), now, fakeGateway{gateway: testGateway()}, db.NewNoOpLockFactory())
	svc.(*service).scanLimit = stickyPageSize

	if err := svc.ReconcileOnce(t.Context()); err != nil {
		t.Fatalf("ReconcileOnce() error = %v", err)
	}

	if got := kc.disableCalls.Load(); got != dueTotal {
		t.Fatalf("disable calls = %d, want %d; drain stopped behind the retained first page", got, dueTotal)
	}
	for i := 0; i < dueTotal; i++ {
		id := fmt.Sprintf("due-%02d", i)
		if status := dao.items[id].Status; status != StatusExpired {
			t.Fatalf("due account %s status = %q, want expired; not reached past the retained page", id, status)
		}
	}
	for i := 0; i < stickyPageSize; i++ {
		id := fmt.Sprintf("sticky-%02d", i)
		if status := dao.items[id].Status; status != StatusProvisioning {
			t.Fatalf("sticky account %s status = %q, want provisioning", id, status)
		}
	}
}

func TestReconcileReportsAndRetriesUndeliveredCredentialCleanup(t *testing.T) {
	dao := newMemoryDAO()
	now := time.Now().UTC()
	account := &OpenShellGatewayServiceAccount{
		GatewayID: "gateway-id", Name: "bot", CreatedByUserID: "creator", Status: StatusError,
		Role: RoleUser, CredentialType: CredentialTypeClientSecret, KeycloakClientID: "client-id",
		KeycloakClientUUID: "client-uuid", ExpiresAt: now.Add(time.Hour),
	}
	dao.seed(account)
	kc := &fakeKeycloak{configured: true, deleteErr: errors.New("provider unavailable")}
	service := newTestService(dao, kc, testBindings("creator", "gateway:viewer"), now)

	if err := service.ReconcileOnce(t.Context()); err == nil {
		t.Fatal("ReconcileOnce() error = nil, want cleanup failure")
	}
	if _, exists := dao.items[account.ID]; !exists {
		t.Fatal("failed cleanup removed the reservation")
	}
	kc.deleteErr = nil
	if err := service.ReconcileOnce(t.Context()); err != nil {
		t.Fatalf("ReconcileOnce() retry error = %v", err)
	}
	if _, exists := dao.items[account.ID]; exists {
		t.Fatal("successful cleanup retained the reservation")
	}
}

func TestReconcileDeletesKeycloakClientsWithoutAResource(t *testing.T) {
	dao := newMemoryDAO()
	kc := &fakeKeycloak{
		configured: true,
		managedClients: []ManagedClient{{
			UUID: "orphan-uuid", GatewayID: "gateway-id", ServiceAccountID: "missing-account",
		}},
	}
	service := newTestService(dao, kc, testBindings("creator", "gateway:viewer"), time.Now().UTC())

	if err := service.ReconcileOnce(t.Context()); err != nil {
		t.Fatalf("ReconcileOnce() error = %v", err)
	}
	if len(kc.deletedUUIDs) != 1 || kc.deletedUUIDs[0] != "orphan-uuid" {
		t.Fatalf("deleted UUIDs = %v", kc.deletedUUIDs)
	}
}

func TestReconcileWithStaleSnapshotNeverReenablesARevokedCredential(t *testing.T) {
	dao := newMemoryDAO()
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	revokedAt := now.Add(-time.Hour)
	account := &OpenShellGatewayServiceAccount{
		GatewayID: "gateway-id", Name: "bot", CreatedByUserID: "creator", Status: StatusRevoked,
		Role: RoleUser, CredentialType: CredentialTypeClientSecret, KeycloakClientID: "client-id",
		KeycloakClientUUID: "client-uuid", Subject: "subject-id", ExpiresAt: now.Add(time.Hour),
		RevokedAt: &revokedAt,
	}
	dao.seed(account)
	// Model a reconciliation scan whose snapshot predates the revocation: it still
	// sees a healthy, not-yet-expired Ready row. The locked reconcile must re-read the
	// committed Revoked row and refuse to re-enable it.
	dao.reconcileSnapshots = []OpenShellGatewayServiceAccount{{
		Meta:      account.Meta,
		GatewayID: "gateway-id", Name: "bot", CreatedByUserID: "creator", Status: StatusReady,
		Role: RoleUser, CredentialType: CredentialTypeClientSecret, KeycloakClientID: "client-id",
		KeycloakClientUUID: "client-uuid", Subject: "subject-id", ExpiresAt: now.Add(time.Hour),
	}}
	kc := &fakeKeycloak{configured: true}
	service := newTestService(dao, kc, testBindings("creator", "gateway:owner"), now)

	if err := service.ReconcileOnce(t.Context()); err != nil {
		t.Fatalf("ReconcileOnce() error = %v", err)
	}
	stored := dao.items[account.ID]
	if stored.Status != StatusRevoked {
		t.Fatalf("stale reconcile changed status to %q, want revoked", stored.Status)
	}
	if kc.reconcileCalls != 0 {
		t.Fatalf("stale reconcile re-enabled the credential: reconcileCalls=%d", kc.reconcileCalls)
	}
	if stored.RevokedAt == nil {
		t.Fatal("stale reconcile cleared the revocation timestamp")
	}
}

func TestReconcileReclaimsAbandonedProvisioningOnlyAfterTheStaleDeadline(t *testing.T) {
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name        string
		updatedAt   time.Time
		wantRemoved bool
	}{
		{name: "recent provisioning is left in flight", updatedAt: now.Add(-time.Minute), wantRemoved: false},
		{name: "stale provisioning is reclaimed", updatedAt: now.Add(-20 * time.Minute), wantRemoved: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			dao := newMemoryDAO()
			account := &OpenShellGatewayServiceAccount{
				GatewayID: "gateway-id", Name: "bot", CreatedByUserID: "creator", Status: StatusProvisioning,
				Role: RoleUser, CredentialType: CredentialTypeClientSecret, KeycloakClientID: "client-id",
				KeycloakClientUUID: "client-uuid", ExpiresAt: now.Add(time.Hour),
			}
			account.CreatedAt = test.updatedAt
			account.UpdatedAt = test.updatedAt
			dao.seed(account)
			kc := &fakeKeycloak{configured: true}
			service := newTestService(dao, kc, testBindings("creator", "gateway:owner"), now)

			if err := service.ReconcileOnce(t.Context()); err != nil {
				t.Fatalf("ReconcileOnce() error = %v", err)
			}
			_, exists := dao.items[account.ID]
			if test.wantRemoved && exists {
				t.Fatalf("stale provisioning row was not reclaimed: %#v", dao.items[account.ID])
			}
			if !test.wantRemoved {
				if !exists {
					t.Fatal("in-flight provisioning row was destroyed")
				}
				if dao.items[account.ID].Status != StatusProvisioning {
					t.Fatalf("in-flight provisioning row mutated to %q", dao.items[account.ID].Status)
				}
				if len(kc.deletedUUIDs) != 0 || kc.deleteManagedCalls != 0 {
					t.Fatalf("in-flight provisioning triggered credential removal: deleted=%v managed=%d", kc.deletedUUIDs, kc.deleteManagedCalls)
				}
			}
		})
	}
}

func TestReconcileRevokesOnlyOnConfirmedGatewayDeletion(t *testing.T) {
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name        string
		gateway     GatewayLookup
		wantRevoked bool
		wantError   bool
		wantReason  string
	}{
		{
			name:        "transient lookup failure keeps the credential",
			gateway:     fakeGateway{serviceErr: trexerrors.GeneralError("control plane down")},
			wantRevoked: false, wantError: true, wantReason: "gateway_unavailable",
		},
		{
			name:        "not-yet-ready OIDC keeps the credential",
			gateway:     fakeGateway{gateway: gatewayWithoutOIDC()},
			wantRevoked: false, wantError: true, wantReason: "gateway_not_ready",
		},
		{
			name:        "confirmed deletion revokes the credential",
			gateway:     fakeGateway{serviceErr: trexerrors.NotFound("gone")},
			wantRevoked: true, wantError: false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			dao := newMemoryDAO()
			account := &OpenShellGatewayServiceAccount{
				GatewayID: "gateway-id", Name: "bot", CreatedByUserID: "creator", Status: StatusReady,
				Role: RoleUser, CredentialType: CredentialTypeClientSecret, KeycloakClientID: "client-id",
				KeycloakClientUUID: "client-uuid", Subject: "subject-id", ExpiresAt: now.Add(time.Hour),
			}
			dao.seed(account)
			kc := &fakeKeycloak{configured: true}
			service := newTestServiceWith(dao, kc, testBindings("creator", "gateway:owner"), now, test.gateway, db.NewNoOpLockFactory())

			err := service.ReconcileOnce(t.Context())
			if test.wantError && err == nil {
				t.Fatal("ReconcileOnce() error = nil, want transient failure surfaced")
			}
			if !test.wantError && err != nil {
				t.Fatalf("ReconcileOnce() error = %v", err)
			}
			stored := dao.items[account.ID]
			if test.wantRevoked {
				if stored.Status != StatusRevoked || stored.RevokedAt == nil {
					t.Fatalf("confirmed deletion did not revoke: %#v", stored)
				}
				return
			}
			if stored.Status != StatusReady {
				t.Fatalf("transient failure changed status to %q, want ready", stored.Status)
			}
			if kc.disableCalls != 0 || len(kc.deletedUUIDs) != 0 {
				t.Fatalf("transient failure wound down the credential: disable=%d deleted=%v", kc.disableCalls, kc.deletedUUIDs)
			}
			if stored.LastError == nil || *stored.LastError != test.wantReason {
				t.Fatalf("last error = %v, want %q", stored.LastError, test.wantReason)
			}
		})
	}
}

func TestReconcileDegradesReadyOnMutationFailureThenRecovers(t *testing.T) {
	dao := newMemoryDAO()
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	account := &OpenShellGatewayServiceAccount{
		GatewayID: "gateway-id", Name: "bot", CreatedByUserID: "creator", Status: StatusReady,
		Role: RoleUser, CredentialType: CredentialTypeClientSecret, KeycloakClientID: "client-id",
		KeycloakClientUUID: "client-uuid", Subject: "subject-id", ExpiresAt: now.Add(time.Hour),
	}
	dao.seed(account)
	kc := &fakeKeycloak{configured: true, reconcileErr: errors.New("provider unavailable")}
	service := newTestService(dao, kc, testBindings("creator", "gateway:owner"), now)

	if err := service.ReconcileOnce(t.Context()); err == nil {
		t.Fatal("ReconcileOnce() error = nil, want converge failure surfaced")
	}
	stored := dao.items[account.ID]
	if stored.Status != StatusDegraded {
		t.Fatalf("failed convergence status = %q, want degraded", stored.Status)
	}
	if stored.LastError == nil || *stored.LastError != "keycloak_reconciliation_failed" {
		t.Fatalf("degraded last error = %v", stored.LastError)
	}

	kc.reconcileErr = nil
	if err := service.ReconcileOnce(t.Context()); err != nil {
		t.Fatalf("ReconcileOnce() recovery error = %v", err)
	}
	stored = dao.items[account.ID]
	if stored.Status != StatusReady || stored.LastError != nil {
		t.Fatalf("recovered account = %#v, want clean ready", stored)
	}
}

func TestLifecycleMutationsSerializeThroughThePerGatewayLock(t *testing.T) {
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	newLockedService := func() (*memoryDAO, *fakeKeycloak, *countingLockFactory, Service) {
		dao := newMemoryDAO()
		kc := &fakeKeycloak{configured: true}
		locks := &countingLockFactory{}
		svc := newTestServiceWith(dao, kc, testBindings("creator", "gateway:owner"), now, fakeGateway{gateway: testGateway()}, locks)
		return dao, kc, locks, svc
	}
	seedReady := func(dao *memoryDAO) *OpenShellGatewayServiceAccount {
		account := &OpenShellGatewayServiceAccount{
			GatewayID: "gateway-id", Name: "bot", CreatedByUserID: "creator", Status: StatusReady,
			Role: RoleUser, CredentialType: CredentialTypeClientSecret, KeycloakClientID: "client-id",
			KeycloakClientUUID: "client-uuid", Subject: "subject-id", ExpiresAt: now.Add(time.Hour),
		}
		dao.seed(account)
		return account
	}

	t.Run("create acquires and releases the lock", func(t *testing.T) {
		_, _, locks, svc := newLockedService()
		if _, problem := svc.Create(t.Context(), "gateway-id", "creator", CreateInput{Name: "bot"}); problem != nil {
			t.Fatalf("Create() problem = %#v", problem)
		}
		assertBalancedLock(t, locks, 1)
	})
	t.Run("revoke acquires and releases the lock", func(t *testing.T) {
		dao, _, locks, svc := newLockedService()
		account := seedReady(dao)
		if _, _, problem := svc.Revoke(t.Context(), "gateway-id", account.ID, "creator"); problem != nil {
			t.Fatalf("Revoke() problem = %#v", problem)
		}
		assertBalancedLock(t, locks, 1)
	})
	t.Run("delete acquires and releases the lock", func(t *testing.T) {
		dao, _, locks, svc := newLockedService()
		account := seedReady(dao)
		if _, _, problem := svc.Delete(t.Context(), "gateway-id", account.ID, "creator"); problem != nil {
			t.Fatalf("Delete() problem = %#v", problem)
		}
		assertBalancedLock(t, locks, 1)
	})
	t.Run("cleanup acquires and releases the lock", func(t *testing.T) {
		dao, _, locks, svc := newLockedService()
		seedReady(dao)
		if err := svc.CleanupGateway(t.Context(), "gateway-id", nil); err != nil {
			t.Fatalf("CleanupGateway() error = %v", err)
		}
		assertBalancedLock(t, locks, 1)
	})
	t.Run("reconcile acquires and releases the lock per account", func(t *testing.T) {
		dao, _, locks, svc := newLockedService()
		seedReady(dao)
		if err := svc.ReconcileOnce(t.Context()); err != nil {
			t.Fatalf("ReconcileOnce() error = %v", err)
		}
		assertBalancedLock(t, locks, 1)
	})
}

func assertBalancedLock(t *testing.T, locks *countingLockFactory, want int) {
	t.Helper()
	if locks.acquired != want {
		t.Fatalf("lock acquisitions = %d, want %d", locks.acquired, want)
	}
	if locks.released != locks.acquired {
		t.Fatalf("lock releases = %d, acquisitions = %d; lock leaked", locks.released, locks.acquired)
	}
}

func gatewayWithoutOIDC() *gateways.Gateway {
	gateway := testGateway()
	gateway.Oidc = nil
	return gateway
}

func newTestService(dao *memoryDAO, kc *fakeKeycloak, bindings fakeBindings, now time.Time) Service {
	result := NewService(dao, fakeGateway{gateway: testGateway()}, bindings, kc, db.NewNoOpLockFactory())
	result.(*service).now = func() time.Time { return now }
	return result
}

// newTestServiceWith builds a service with an explicit gateway lookup and lock
// factory so tests can inject transient/404 gateway failures or count locking.
func newTestServiceWith(dao *memoryDAO, kc ServiceAccountProvisioner, bindings fakeBindings, now time.Time, gateway GatewayLookup, lockFactory db.LockFactory) Service {
	result := NewService(dao, gateway, bindings, kc, lockFactory)
	result.(*service).now = func() time.Time { return now }
	return result
}

func testGateway() *gateways.Gateway {
	oidc, _ := json.Marshal(GatewayOIDC{Issuer: "https://issuer.example/realms/hypershell", ClientID: "gateway-client", Audience: "gateway-client"})
	phase, status, route := "Running", "Healthy", "grpcs://gateway.example:443"
	gateway := &gateways.Gateway{Name: "gateway", Phase: &phase, Status: &status, RouteAddress: &route, Oidc: stringPointer(string(oidc))}
	gateway.ID = "gateway-id"
	return gateway
}

// countingLockFactory records how many times the per-gateway advisory lock is
// acquired and released so tests can assert that every lifecycle path serializes
// through it. It otherwise behaves as a no-op, matching NoOpLockFactory.
type countingLockFactory struct {
	acquired int
	released int
}

func (f *countingLockFactory) NewAdvisoryLock(_ context.Context, id string, _ db.LockType) (string, error) {
	f.acquired++
	return id, nil
}

func (f *countingLockFactory) NewNonBlockingLock(_ context.Context, id string, _ db.LockType) (string, bool, error) {
	f.acquired++
	return id, true, nil
}

func (f *countingLockFactory) Unlock(_ context.Context, _ string) { f.released++ }

// blockingLockFactory serializes acquisition per id with a real mutex so a test
// can prove one lifecycle path genuinely blocks another until the lock is
// released, rather than merely counting acquisitions.
type blockingLockFactory struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func newBlockingLockFactory() *blockingLockFactory {
	return &blockingLockFactory{locks: map[string]*sync.Mutex{}}
}

func (f *blockingLockFactory) lockFor(id string) *sync.Mutex {
	f.mu.Lock()
	defer f.mu.Unlock()
	lock, ok := f.locks[id]
	if !ok {
		lock = &sync.Mutex{}
		f.locks[id] = lock
	}
	return lock
}

func (f *blockingLockFactory) NewAdvisoryLock(_ context.Context, id string, _ db.LockType) (string, error) {
	f.lockFor(id).Lock()
	return id, nil
}

func (f *blockingLockFactory) NewNonBlockingLock(_ context.Context, id string, _ db.LockType) (string, bool, error) {
	if f.lockFor(id).TryLock() {
		return id, true, nil
	}
	return id, false, nil
}

func (f *blockingLockFactory) Unlock(_ context.Context, id string) { f.lockFor(id).Unlock() }

// mutableGateway is a GatewayLookup whose gateway can be removed mid-test so a
// finalize callback can model the gateway-row deletion that a blocked Create
// must observe once it acquires the lock.
type mutableGateway struct {
	mu      sync.Mutex
	gateway *gateways.Gateway
}

func (g *mutableGateway) Get(_ context.Context, id string) (*gateways.Gateway, *trexerrors.ServiceError) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.gateway == nil || g.gateway.ID != id {
		return nil, trexerrors.NotFound("not found")
	}
	return g.gateway, nil
}

func (g *mutableGateway) delete() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.gateway = nil
}

type fakeGateway struct {
	gateway *gateways.Gateway
	// serviceErr, when set, is returned by Get in place of the gateway. It models a
	// transient control-plane failure (e.g. a 500) or a not-yet-ready gateway (409)
	// so reconciliation's confirmed-deletion-versus-transient branch can be exercised.
	serviceErr *trexerrors.ServiceError
}

func (f fakeGateway) Get(_ context.Context, id string) (*gateways.Gateway, *trexerrors.ServiceError) {
	if f.serviceErr != nil {
		return nil, f.serviceErr
	}
	if f.gateway == nil || f.gateway.ID != id {
		return nil, trexerrors.NotFound("not found")
	}
	return f.gateway, nil
}

type fakeBindings struct {
	byUser map[string][]rbac.BindingSummary
}

func (f fakeBindings) FindBindingsByUserID(_ context.Context, userID string) ([]rbac.BindingSummary, error) {
	return f.byUser[userID], nil
}

func testBindings(userID, role string) fakeBindings {
	return fakeBindings{byUser: map[string][]rbac.BindingSummary{userID: binding(role)}}
}

func binding(role string) []rbac.BindingSummary {
	gatewayID := "gateway-id"
	return []rbac.BindingSummary{{RoleName: role, Scope: "gateway", GatewayID: &gatewayID}}
}

type fakeKeycloak struct {
	configured         bool
	disableErr         error
	deleteErr          error
	provisionErr       error
	reconcileErr       error
	secret             string
	lastSpec           ProvisioningSpec
	managedClients     []ManagedClient
	provisionCalls     int
	reconcileCalls     int
	disableCalls       int
	deleteManagedCalls int
	deleteGatewayCalls int
	deletedUUIDs       []string
	lastOwnership      ownershipArgs
}

// ownershipArgs captures the identifiers passed to the last Disable/Delete call
// so tests can assert the exact gateway and service-account ownership metadata is
// threaded through to the provisioner.
type ownershipArgs struct {
	UUID             string
	GatewayID        string
	ServiceAccountID string
}

func (f *fakeKeycloak) Configured() bool { return f.configured }

func (f *fakeKeycloak) ProvisionServiceAccount(_ context.Context, spec ProvisioningSpec) (*ProvisionedServiceAccount, error) {
	f.provisionCalls++
	f.lastSpec = spec
	if f.provisionErr != nil {
		return nil, f.provisionErr
	}
	if f.secret == "" {
		f.secret = "one-time-secret"
	}
	return &ProvisionedServiceAccount{ClientUUID: "client-uuid", ClientID: spec.ClientID, ClientSecret: f.secret, Subject: "subject-id"}, nil
}

func (f *fakeKeycloak) ReconcileServiceAccount(_ context.Context, spec ProvisioningSpec, _, _ string, _ bool) error {
	f.reconcileCalls++
	f.lastSpec = spec
	return f.reconcileErr
}

func (f *fakeKeycloak) DisableServiceAccount(_ context.Context, uuid, gatewayID, serviceAccountID string) error {
	f.disableCalls++
	f.lastOwnership = ownershipArgs{UUID: uuid, GatewayID: gatewayID, ServiceAccountID: serviceAccountID}
	return f.disableErr
}

func (f *fakeKeycloak) DeleteServiceAccount(_ context.Context, uuid, gatewayID, serviceAccountID string) error {
	f.lastOwnership = ownershipArgs{UUID: uuid, GatewayID: gatewayID, ServiceAccountID: serviceAccountID}
	if f.deleteErr == nil {
		f.deletedUUIDs = append(f.deletedUUIDs, uuid)
	}
	return f.deleteErr
}
func (f *fakeKeycloak) DeleteManagedServiceAccount(context.Context, string, string) error {
	f.deleteManagedCalls++
	return f.deleteErr
}
func (f *fakeKeycloak) DeleteGatewayServiceAccounts(context.Context, string) error {
	f.deleteGatewayCalls++
	return f.deleteErr
}
func (f *fakeKeycloak) ListManagedClients(context.Context, string) ([]ManagedClient, error) {
	return f.managedClients, nil
}

// latencyKeycloak is a concurrency-safe provisioner that adds a fixed delay to
// each disablement and records the peak number of concurrent disablements so a
// test can prove due work is drained in parallel rather than serialized.
type latencyKeycloak struct {
	delay        time.Duration
	inFlight     atomic.Int32
	maxInFlight  atomic.Int32
	disableCalls atomic.Int32
}

func (k *latencyKeycloak) Configured() bool { return true }

func (k *latencyKeycloak) ProvisionServiceAccount(context.Context, ProvisioningSpec) (*ProvisionedServiceAccount, error) {
	return nil, errors.New("unexpected provision during reconciliation")
}

func (k *latencyKeycloak) ReconcileServiceAccount(context.Context, ProvisioningSpec, string, string, bool) error {
	return nil
}

func (k *latencyKeycloak) DisableServiceAccount(context.Context, string, string, string) error {
	n := k.inFlight.Add(1)
	for {
		peak := k.maxInFlight.Load()
		if n <= peak || k.maxInFlight.CompareAndSwap(peak, n) {
			break
		}
	}
	time.Sleep(k.delay)
	k.inFlight.Add(-1)
	k.disableCalls.Add(1)
	return nil
}

func (k *latencyKeycloak) DeleteServiceAccount(context.Context, string, string, string) error {
	return nil
}
func (k *latencyKeycloak) DeleteManagedServiceAccount(context.Context, string, string) error {
	return nil
}
func (k *latencyKeycloak) DeleteGatewayServiceAccounts(context.Context, string) error { return nil }
func (k *latencyKeycloak) ListManagedClients(context.Context, string) ([]ManagedClient, error) {
	return nil, nil
}

type memoryDAO struct {
	// mu guards every field so concurrent reconciliation workers exercise the fake
	// safely, mirroring the pool safety of the production DAO.
	mu     sync.Mutex
	items  map[string]*OpenShellGatewayServiceAccount
	audits []*AuditEvent
	next   int
	// reconcileSnapshots, when set, is returned by ListDueAndTransitional in place of
	// live rows. It lets tests model a stale scan snapshot that disagrees with the
	// committed row a locked reconciliation re-reads.
	reconcileSnapshots []OpenShellGatewayServiceAccount
}

func newMemoryDAO() *memoryDAO {
	return &memoryDAO{items: map[string]*OpenShellGatewayServiceAccount{}}
}

func (d *memoryDAO) seed(accounts ...*OpenShellGatewayServiceAccount) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, account := range accounts {
		if account.ID == "" {
			d.next++
			account.ID = "account-" + string(rune('0'+d.next))
		}
		if account.CreatedAt.IsZero() {
			account.CreatedAt = time.Now().UTC()
			account.UpdatedAt = account.CreatedAt
		}
		copy := *account
		d.items[account.ID] = &copy
	}
}

func (d *memoryDAO) ActiveNameExists(_ context.Context, gatewayID, name string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, account := range d.items {
		if account.GatewayID == gatewayID && strings.EqualFold(account.Name, name) && account.Status != StatusExpired && account.Status != StatusRevoked {
			return true, nil
		}
	}
	return false, nil
}

func (d *memoryDAO) Create(_ context.Context, account *OpenShellGatewayServiceAccount) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, exists := d.items[account.ID]; exists {
		return errors.New("duplicate")
	}
	account.CreatedAt = time.Now().UTC()
	account.UpdatedAt = account.CreatedAt
	copy := *account
	d.items[account.ID] = &copy
	return nil
}

func (d *memoryDAO) Update(_ context.Context, account *OpenShellGatewayServiceAccount) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	account.UpdatedAt = time.Now().UTC()
	copy := *account
	d.items[account.ID] = &copy
	return nil
}

func (d *memoryDAO) ConditionalUpdate(_ context.Context, account *OpenShellGatewayServiceAccount, expectedStatus string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	stored, ok := d.items[account.ID]
	if !ok || stored.Status != expectedStatus {
		return false, nil
	}
	account.UpdatedAt = time.Now().UTC()
	copy := *account
	d.items[account.ID] = &copy
	return true, nil
}

func (d *memoryDAO) Get(_ context.Context, gatewayID, id string) (*OpenShellGatewayServiceAccount, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	account, ok := d.items[id]
	if !ok || account.GatewayID != gatewayID {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *account
	return &copy, nil
}

func (d *memoryDAO) List(_ context.Context, gatewayID string, options ListOptions) ([]OpenShellGatewayServiceAccount, int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	items := make([]OpenShellGatewayServiceAccount, 0)
	for _, account := range d.items {
		if account.GatewayID != gatewayID || (options.CreatorUserID != "" && account.CreatedByUserID != options.CreatorUserID) || (options.Status != "" && account.Status != options.Status) {
			continue
		}
		if options.Search != "" && !strings.Contains(strings.ToLower(account.Name), strings.ToLower(options.Search)) {
			continue
		}
		items = append(items, *account)
	}
	return items, int64(len(items)), nil
}

func (d *memoryDAO) CountActive(_ context.Context, gatewayID, creator string) (int64, int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var gatewayCount, creatorCount int64
	for _, account := range d.items {
		if account.GatewayID == gatewayID && account.Status != StatusExpired && account.Status != StatusRevoked {
			gatewayCount++
			if account.CreatedByUserID == creator {
				creatorCount++
			}
		}
	}
	return gatewayCount, creatorCount, nil
}

func (d *memoryDAO) ListDueAndTransitional(_ context.Context, now time.Time, after ReconcileCursor, limit int) ([]OpenShellGatewayServiceAccount, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var items []OpenShellGatewayServiceAccount
	if d.reconcileSnapshots != nil {
		items = append(items, d.reconcileSnapshots...)
	} else {
		transitional := map[string]bool{
			StatusProvisioning: true, StatusRevoking: true, StatusDeleting: true,
			StatusError: true, StatusDegraded: true,
		}
		for _, account := range d.items {
			if transitional[account.Status] || (account.Status == StatusReady && !account.ExpiresAt.After(now)) {
				items = append(items, *account)
			}
		}
	}
	// Mirror the SQL keyset: order by (expires_at, id), then serve the rows
	// strictly after the cursor, capped at the page limit. This makes the fake
	// advance page by page exactly as the production DAO does, so a drain over a
	// backlog larger than one page terminates in the test double too.
	sort.Slice(items, func(i, j int) bool {
		if !items[i].ExpiresAt.Equal(items[j].ExpiresAt) {
			return items[i].ExpiresAt.Before(items[j].ExpiresAt)
		}
		return items[i].ID < items[j].ID
	})
	page := make([]OpenShellGatewayServiceAccount, 0, len(items))
	for _, account := range items {
		if !after.isStart() && !afterCursor(account, after) {
			continue
		}
		page = append(page, account)
		if limit > 0 && len(page) == limit {
			break
		}
	}
	return page, nil
}

// afterCursor reports whether account sorts strictly after the keyset cursor over
// (expires_at, id), matching the SQL row-value comparison.
func afterCursor(account OpenShellGatewayServiceAccount, cursor ReconcileCursor) bool {
	if !account.ExpiresAt.Equal(cursor.ExpiresAt) {
		return account.ExpiresAt.After(cursor.ExpiresAt)
	}
	return account.ID > cursor.ID
}

func (d *memoryDAO) ListDrift(_ context.Context, now time.Time, _ int) ([]OpenShellGatewayServiceAccount, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.reconcileSnapshots != nil {
		return nil, nil
	}
	items := make([]OpenShellGatewayServiceAccount, 0, len(d.items))
	for _, account := range d.items {
		if (account.Status == StatusReady && account.ExpiresAt.After(now)) ||
			account.Status == StatusExpired || account.Status == StatusRevoked {
			items = append(items, *account)
		}
	}
	return items, nil
}

func (d *memoryDAO) SoftDelete(_ context.Context, account *OpenShellGatewayServiceAccount) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.items, account.ID)
	return nil
}

func (d *memoryDAO) CreateAudit(_ context.Context, event *AuditEvent) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	copy := *event
	d.audits = append(d.audits, &copy)
	return nil
}

func itemsIDByCreator(dao *memoryDAO, creator string) string {
	for id, account := range dao.items {
		if account.CreatedByUserID == creator {
			return id
		}
	}
	return ""
}

func stringPointer(value string) *string { return &value }
