package serviceAccounts

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/openshift-online/hypershell/components/api-server/plugins/defaultWorkspaceMemberships"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
	trexerrors "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// fakeMemberships is an in-memory defaultWorkspaceMemberships.MembershipService
// for OSG-SACC-19 unit tests. Rows are keyed by gateway_id/subject, mirroring
// the real service's (gateway_id, subject) uniqueness.
type fakeMemberships struct {
	mu    sync.Mutex
	items map[string]*defaultWorkspaceMemberships.DefaultWorkspaceMembership
	// countOverride, when non-nil, is returned by CountLiveForGateway instead of
	// the live item count, so a test can simulate the gateway already being at
	// the 1,000-member limit without seeding 1,000 rows.
	countOverride *int64
	createCalls   []defaultWorkspaceMemberships.DefaultWorkspaceMembership
	deleteCalls   []string // "gatewayID/subject"
	// createErr, when non-nil, is returned by Create instead of succeeding, so a
	// test can exercise the membership-convergence failure path.
	createErr *trexerrors.ServiceError
}

func newFakeMemberships() *fakeMemberships {
	return &fakeMemberships{items: map[string]*defaultWorkspaceMemberships.DefaultWorkspaceMembership{}}
}

func membershipKey(gatewayID, subject string) string { return gatewayID + "/" + subject }

func (f *fakeMemberships) Get(ctx context.Context, id string) (*defaultWorkspaceMemberships.DefaultWorkspaceMembership, *trexerrors.ServiceError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, item := range f.items {
		if item.ID == id {
			return item, nil
		}
	}
	return nil, trexerrors.NotFound("not found")
}

func (f *fakeMemberships) GetUnscoped(ctx context.Context, id string) (*defaultWorkspaceMemberships.DefaultWorkspaceMembership, *trexerrors.ServiceError) {
	return f.Get(ctx, id)
}

func (f *fakeMemberships) Create(ctx context.Context, gatewayID, subject, source string) (*defaultWorkspaceMemberships.DefaultWorkspaceMembership, *trexerrors.ServiceError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createErr != nil {
		return nil, f.createErr
	}
	key := membershipKey(gatewayID, subject)
	if existing, ok := f.items[key]; ok {
		return existing, nil
	}
	item := &defaultWorkspaceMemberships.DefaultWorkspaceMembership{
		GatewayID: gatewayID, Subject: subject, Role: defaultWorkspaceMemberships.RoleUser,
		Source: source, ReconciliationStatus: defaultWorkspaceMemberships.StatusPending,
	}
	item.ID = "membership-" + key
	f.items[key] = item
	f.createCalls = append(f.createCalls, *item)
	return item, nil
}

func (f *fakeMemberships) DeleteByGatewayAndSubject(ctx context.Context, gatewayID, subject string) *trexerrors.ServiceError {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := membershipKey(gatewayID, subject)
	delete(f.items, key)
	f.deleteCalls = append(f.deleteCalls, key)
	return nil
}

func (f *fakeMemberships) FindByGatewayAndSubject(ctx context.Context, gatewayID, subject string) (*defaultWorkspaceMemberships.DefaultWorkspaceMembership, *trexerrors.ServiceError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.items[membershipKey(gatewayID, subject)], nil
}

func (f *fakeMemberships) CountLiveForGateway(ctx context.Context, gatewayID string) (int64, *trexerrors.ServiceError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.countOverride != nil {
		return *f.countOverride, nil
	}
	var count int64
	for _, item := range f.items {
		if item.GatewayID == gatewayID {
			count++
		}
	}
	return count, nil
}

func (f *fakeMemberships) AllForGateway(ctx context.Context, gatewayID string) (defaultWorkspaceMemberships.DefaultWorkspaceMembershipList, *trexerrors.ServiceError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out defaultWorkspaceMemberships.DefaultWorkspaceMembershipList
	for _, item := range f.items {
		if item.GatewayID == gatewayID {
			out = append(out, item)
		}
	}
	return out, nil
}

func (f *fakeMemberships) DeleteAllForGateway(ctx context.Context, gatewayID string) (int64, *trexerrors.ServiceError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var count int64
	for key, item := range f.items {
		if item.GatewayID == gatewayID {
			delete(f.items, key)
			count++
		}
	}
	return count, nil
}

func (f *fakeMemberships) FindByIDs(ctx context.Context, ids []string) (defaultWorkspaceMemberships.DefaultWorkspaceMembershipList, *trexerrors.ServiceError) {
	return nil, nil
}

func (f *fakeMemberships) All(ctx context.Context) (defaultWorkspaceMemberships.DefaultWorkspaceMembershipList, *trexerrors.ServiceError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out defaultWorkspaceMemberships.DefaultWorkspaceMembershipList
	for _, item := range f.items {
		out = append(out, item)
	}
	return out, nil
}

func (f *fakeMemberships) CountByStatus(ctx context.Context) (map[string]int64, *trexerrors.ServiceError) {
	return nil, nil
}

func (f *fakeMemberships) UpdateStatus(ctx context.Context, id, status string, lastReconcileError *string) (*defaultWorkspaceMemberships.DefaultWorkspaceMembership, *trexerrors.ServiceError) {
	return nil, nil
}

var _ defaultWorkspaceMemberships.MembershipService = &fakeMemberships{}

func newTestServiceWithMemberships(dao *memoryDAO, kc *fakeKeycloak, bindings fakeBindings, now time.Time, memberships defaultWorkspaceMemberships.MembershipService) Service {
	result := NewService(dao, fakeGateway{gateway: testGateway()}, bindings, kc, db.NewNoOpLockFactory(), memberships)
	result.(*service).now = func() time.Time { return now }
	return result
}

// Verifies: OSG-SACC-19.01
func TestCreate_OpenShellUserGetsDefaultWorkspaceMembership(t *testing.T) {
	dao := newMemoryDAO()
	memberships := newFakeMemberships()
	service := newTestServiceWithMemberships(dao, &fakeKeycloak{configured: true}, testBindings("creator", "gateway:owner"), time.Now().UTC(), memberships)

	result, problem := service.Create(t.Context(), "gateway-id", "creator", CreateInput{Name: "bot", Role: RoleUser})
	if problem != nil {
		t.Fatalf("Create() problem = %#v", problem)
	}
	item, found := memberships.items[membershipKey("gateway-id", result.Account.Subject)]
	if !found {
		t.Fatalf("expected a default workspace membership for subject %q, got %#v", result.Account.Subject, memberships.items)
	}
	if item.Source != defaultWorkspaceMemberships.SourceServiceAccount {
		t.Fatalf("source = %q, want %q", item.Source, defaultWorkspaceMemberships.SourceServiceAccount)
	}
	if item.Role != defaultWorkspaceMemberships.RoleUser {
		t.Fatalf("role = %q, want %q", item.Role, defaultWorkspaceMemberships.RoleUser)
	}
}

// Verifies: OSG-SACC-19.02
func TestCreate_OpenShellAdminGetsNoDefaultWorkspaceMembership(t *testing.T) {
	dao := newMemoryDAO()
	memberships := newFakeMemberships()
	service := newTestServiceWithMemberships(dao, &fakeKeycloak{configured: true}, testBindings("creator", "gateway:owner"), time.Now().UTC(), memberships)

	if _, problem := service.Create(t.Context(), "gateway-id", "creator", CreateInput{Name: "admin-bot", Role: RoleAdmin}); problem != nil {
		t.Fatalf("Create() problem = %#v", problem)
	}
	if len(memberships.items) != 0 {
		t.Fatalf("expected no default workspace membership for an openshell-admin account, got %#v", memberships.items)
	}
}

// Verifies: OSG-SACC-19.03
func TestCreate_RejectedAtDefaultWorkspaceMemberLimit(t *testing.T) {
	dao := newMemoryDAO()
	memberships := newFakeMemberships()
	limit := int64(defaultWorkspaceMemberships.MemberLimit)
	memberships.countOverride = &limit
	kc := &fakeKeycloak{configured: true}
	service := newTestServiceWithMemberships(dao, kc, testBindings("creator", "gateway:owner"), time.Now().UTC(), memberships)

	_, problem := service.Create(t.Context(), "gateway-id", "creator", CreateInput{Name: "bot", Role: RoleUser})
	if problem == nil || problem.Status != 409 {
		t.Fatalf("Create() problem = %#v, want 409", problem)
	}
	if kc.provisionCalls != 0 {
		t.Fatalf("expected no Keycloak provisioning, got %d calls", kc.provisionCalls)
	}
	if len(dao.items) != 0 {
		t.Fatalf("expected no OpenShellGatewayServiceAccount record, got %#v", dao.items)
	}
	if len(memberships.createCalls) != 0 {
		t.Fatalf("expected no DefaultWorkspaceMembership row, got %#v", memberships.createCalls)
	}
}

// Verifies: OSG-SACC-19.04
func TestRevoke_DeletesDefaultWorkspaceMembership(t *testing.T) {
	dao := newMemoryDAO()
	memberships := newFakeMemberships()
	service := newTestServiceWithMemberships(dao, &fakeKeycloak{configured: true}, testBindings("creator", "gateway:owner"), time.Now().UTC(), memberships)

	result, problem := service.Create(t.Context(), "gateway-id", "creator", CreateInput{Name: "bot", Role: RoleUser})
	if problem != nil {
		t.Fatalf("Create() problem = %#v", problem)
	}
	if len(memberships.items) != 1 {
		t.Fatalf("expected the membership to exist before revoke, got %#v", memberships.items)
	}
	if _, _, problem := service.Revoke(t.Context(), "gateway-id", result.Account.ID, "creator"); problem != nil {
		t.Fatalf("Revoke() problem = %#v", problem)
	}
	if len(memberships.items) != 0 {
		t.Fatalf("expected the membership to be deleted after revoke, got %#v", memberships.items)
	}
}

// Verifies: OSG-SACC-19.07, OSG-SACC-19.08
func TestUpdateRole_ConvergesDefaultWorkspaceMembershipBothWays(t *testing.T) {
	dao := newMemoryDAO()
	now := time.Now().UTC()
	account := &OpenShellGatewayServiceAccount{
		GatewayID: "gateway-id", Name: "bot", CreatedByUserID: "creator", Status: StatusReady,
		Role: RoleUser, CredentialType: CredentialTypeClientSecret, KeycloakClientID: "client-id",
		KeycloakClientUUID: "client-uuid", Subject: "subject-id", ExpiresAt: now.Add(time.Hour),
	}
	dao.seed(account)
	memberships := newFakeMemberships()
	if _, serr := memberships.Create(context.Background(), "gateway-id", "subject-id", defaultWorkspaceMemberships.SourceServiceAccount); serr != nil {
		t.Fatalf("seed membership: %v", serr)
	}
	service := newTestServiceWithMemberships(dao, &fakeKeycloak{configured: true}, testBindings("creator", "gateway:owner"), now, memberships)

	// user -> admin: membership removed.
	if _, _, problem := service.UpdateRole(t.Context(), "gateway-id", account.ID, "creator", RoleUpdateInput{Role: RoleAdmin}); problem != nil {
		t.Fatalf("UpdateRole() to admin problem = %#v", problem)
	}
	if len(memberships.items) != 0 {
		t.Fatalf("expected membership removed after upgrade to admin, got %#v", memberships.items)
	}

	// admin -> user: membership created, subject to the limit.
	if _, _, problem := service.UpdateRole(t.Context(), "gateway-id", account.ID, "creator", RoleUpdateInput{Role: RoleUser}); problem != nil {
		t.Fatalf("UpdateRole() to user problem = %#v", problem)
	}
	if _, found := memberships.items[membershipKey("gateway-id", "subject-id")]; !found {
		t.Fatalf("expected membership recreated after downgrade to user, got %#v", memberships.items)
	}
}

// Reject at limit.
// Verifies: OSG-SACC-19.08
func TestUpdateRole_DowngradeToUserRejectedAtLimit(t *testing.T) {
	dao := newMemoryDAO()
	now := time.Now().UTC()
	account := &OpenShellGatewayServiceAccount{
		GatewayID: "gateway-id", Name: "bot", CreatedByUserID: "creator", Status: StatusReady,
		Role: RoleAdmin, CredentialType: CredentialTypeClientSecret, KeycloakClientID: "client-id",
		KeycloakClientUUID: "client-uuid", Subject: "subject-id", ExpiresAt: now.Add(time.Hour),
	}
	dao.seed(account)
	memberships := newFakeMemberships()
	limit := int64(defaultWorkspaceMemberships.MemberLimit)
	memberships.countOverride = &limit
	service := newTestServiceWithMemberships(dao, &fakeKeycloak{configured: true}, testBindings("creator", "gateway:owner"), now, memberships)

	_, _, problem := service.UpdateRole(t.Context(), "gateway-id", account.ID, "creator", RoleUpdateInput{Role: RoleUser})
	if problem == nil || problem.Status != 409 {
		t.Fatalf("UpdateRole() problem = %#v, want 409", problem)
	}
	stored := dao.items[account.ID]
	if stored.Role != RoleAdmin {
		t.Fatalf("expected role unchanged at %q, got %q", RoleAdmin, stored.Role)
	}
}

// Verifies: OSG-SACC-19.06
func TestReconcile_ExpirationDeletesDefaultWorkspaceMembership(t *testing.T) {
	dao := newMemoryDAO()
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	account := &OpenShellGatewayServiceAccount{
		GatewayID: "gateway-id", Name: "bot", CreatedByUserID: "creator", Status: StatusReady,
		Role: RoleUser, CredentialType: CredentialTypeClientSecret, KeycloakClientID: "client-id",
		KeycloakClientUUID: "client-uuid", Subject: "subject-id", ExpiresAt: now.Add(-time.Minute),
	}
	dao.seed(account)
	memberships := newFakeMemberships()
	if _, serr := memberships.Create(context.Background(), "gateway-id", "subject-id", defaultWorkspaceMemberships.SourceServiceAccount); serr != nil {
		t.Fatalf("seed membership: %v", serr)
	}
	service := newTestServiceWithMemberships(dao, &fakeKeycloak{configured: true}, testBindings("creator", "gateway:owner"), now, memberships)

	if err := service.ReconcileOnce(t.Context()); err != nil {
		t.Fatalf("ReconcileOnce() error = %v", err)
	}
	if dao.items[account.ID].Status != StatusExpired {
		t.Fatalf("expected account expired, got %#v", dao.items[account.ID])
	}
	if len(memberships.items) != 0 {
		t.Fatalf("expected membership deleted on expiration, got %#v", memberships.items)
	}
}

// Verifies the forced-downgrade reconcile path also converges membership
// (service.go reconcileOne, "downgraded" branch).
func TestReconcile_ForcedDowngradeCreatesDefaultWorkspaceMembership(t *testing.T) {
	dao := newMemoryDAO()
	now := time.Now().UTC()
	account := &OpenShellGatewayServiceAccount{
		GatewayID: "gateway-id", Name: "bot", CreatedByUserID: "creator", Status: StatusReady,
		Role: RoleAdmin, CredentialType: CredentialTypeClientSecret, KeycloakClientID: "client-id",
		KeycloakClientUUID: "client-uuid", Subject: "subject-id", ExpiresAt: now.Add(time.Hour),
	}
	dao.seed(account)
	bindings := testBindings("creator", "gateway:viewer")
	memberships := newFakeMemberships()
	service := newTestServiceWithMemberships(dao, &fakeKeycloak{configured: true}, bindings, now, memberships)

	if err := service.ReconcileOnce(t.Context()); err != nil {
		t.Fatalf("ReconcileOnce() error = %v", err)
	}
	stored := dao.items[account.ID]
	if stored.Role != RoleUser {
		t.Fatalf("expected forced downgrade to RoleUser, got %#v", stored)
	}
	if _, found := memberships.items[membershipKey("gateway-id", "subject-id")]; !found {
		t.Fatalf("expected a default workspace membership created on forced downgrade, got %#v", memberships.items)
	}
}

// A transient membership-create failure during UpdateRole must fail the whole
// request and revert the persisted role: there is no later reconcile pass that
// retries membership convergence for an already-Ready account, so silently
// continuing would leave the account permanently in RoleUser with no
// membership.
func TestUpdateRole_RevertsRoleWhenMembershipConvergenceFails(t *testing.T) {
	dao := newMemoryDAO()
	now := time.Now().UTC()
	account := &OpenShellGatewayServiceAccount{
		GatewayID: "gateway-id", Name: "bot", CreatedByUserID: "creator", Status: StatusReady,
		Role: RoleAdmin, CredentialType: CredentialTypeClientSecret, KeycloakClientID: "client-id",
		KeycloakClientUUID: "client-uuid", Subject: "subject-id", ExpiresAt: now.Add(time.Hour),
	}
	dao.seed(account)
	memberships := newFakeMemberships()
	memberships.createErr = trexerrors.GeneralError("transient failure")
	kc := &fakeKeycloak{configured: true}
	service := newTestServiceWithMemberships(dao, kc, testBindings("creator", "gateway:owner"), now, memberships)

	_, _, problem := service.UpdateRole(t.Context(), "gateway-id", account.ID, "creator", RoleUpdateInput{Role: RoleUser})
	if problem == nil || problem.Status != 500 {
		t.Fatalf("UpdateRole() problem = %#v, want 500", problem)
	}
	stored := dao.items[account.ID]
	if stored.Role != RoleAdmin {
		t.Fatalf("expected role reverted to %q, got %q", RoleAdmin, stored.Role)
	}
	if kc.reconcileCalls != 0 {
		t.Fatalf("expected no Keycloak call before the membership convergence failure, got %d", kc.reconcileCalls)
	}
}

// Verifies the forced-downgrade reconcile path retries membership creation on
// a later pass rather than persisting RoleUser with no membership and no
// future retry opportunity.
func TestReconcile_ForcedDowngradeRetriesMembershipCreateAfterTransientFailure(t *testing.T) {
	dao := newMemoryDAO()
	now := time.Now().UTC()
	account := &OpenShellGatewayServiceAccount{
		GatewayID: "gateway-id", Name: "bot", CreatedByUserID: "creator", Status: StatusReady,
		Role: RoleAdmin, CredentialType: CredentialTypeClientSecret, KeycloakClientID: "client-id",
		KeycloakClientUUID: "client-uuid", Subject: "subject-id", ExpiresAt: now.Add(time.Hour),
	}
	dao.seed(account)
	bindings := testBindings("creator", "gateway:viewer")
	memberships := newFakeMemberships()
	memberships.createErr = trexerrors.GeneralError("transient failure")
	service := newTestServiceWithMemberships(dao, &fakeKeycloak{configured: true}, bindings, now, memberships)

	if err := service.ReconcileOnce(t.Context()); err == nil {
		t.Fatal("expected ReconcileOnce to surface the membership create failure")
	}
	stored := dao.items[account.ID]
	if stored.Role != RoleAdmin {
		t.Fatalf("expected role to remain %q so the next pass retries the downgrade, got %q", RoleAdmin, stored.Role)
	}
	if len(memberships.items) != 0 {
		t.Fatalf("expected no membership persisted after the failed create, got %#v", memberships.items)
	}

	// The next pass retries with the transient failure cleared and succeeds.
	memberships.createErr = nil
	if err := service.ReconcileOnce(t.Context()); err != nil {
		t.Fatalf("ReconcileOnce() retry error = %v", err)
	}
	stored = dao.items[account.ID]
	if stored.Role != RoleUser {
		t.Fatalf("expected role downgraded to %q after the retry succeeds, got %q", RoleUser, stored.Role)
	}
	if _, found := memberships.items[membershipKey("gateway-id", "subject-id")]; !found {
		t.Fatalf("expected the membership created on retry, got %#v", memberships.items)
	}
}

// Regression test: Create() can fail AFTER the default-workspace membership
// row is already durably created -- here, the "create succeeded" audit write
// fails right after a successful membership create. That failure marks the
// account StatusError without undoing the membership. The background
// reconciler's StatusError cleanup path (cleanupUndeliveredAccount) must also
// delete the now-orphaned membership, not just the account: otherwise it
// would survive indefinitely, permanently consuming a slot against the
// gateway's 1,000-member OpenShell cap.
func TestCreate_AuditFailureAfterMembershipCreateDoesNotLeakMembership(t *testing.T) {
	dao := newMemoryDAO()
	dao.auditFailOnOutcome = "succeeded"
	memberships := newFakeMemberships()
	kc := &fakeKeycloak{configured: true}
	service := newTestServiceWithMemberships(dao, kc, testBindings("creator", "gateway:owner"), time.Now().UTC(), memberships)

	if _, problem := service.Create(t.Context(), "gateway-id", "creator", CreateInput{Name: "bot", Role: RoleUser}); problem == nil {
		t.Fatal("expected Create() to fail when the post-membership audit write fails")
	}

	// The account landed in StatusError with the membership already created.
	if len(dao.items) != 1 {
		t.Fatalf("expected exactly one account, got %#v", dao.items)
	}
	var stored *OpenShellGatewayServiceAccount
	for _, a := range dao.items {
		stored = a
	}
	if stored.Status != StatusError {
		t.Fatalf("expected the account in StatusError, got %#v", stored)
	}
	if len(memberships.items) != 1 {
		t.Fatalf("expected the membership created before the audit failure to still exist, got %#v", memberships.items)
	}

	// The background reconciler's StatusError cleanup path must also remove
	// the orphaned membership.
	if err := service.ReconcileOnce(t.Context()); err != nil {
		t.Fatalf("ReconcileOnce() error = %v", err)
	}
	if len(dao.items) != 0 {
		t.Fatalf("expected the undelivered account to be soft-deleted, got %#v", dao.items)
	}
	if len(memberships.items) != 0 {
		t.Fatalf("expected the orphaned default workspace membership to be deleted, got %#v", memberships.items)
	}
}

// Regression test for the cross-plugin race on the GTW-ACC-17/OSG-SACC-19
// 1,000-member limit: a concurrent gatewayAccess grant and serviceAccounts
// create for the same gateway must serialize through the SAME advisory-lock
// domain, or both could observe the live membership count below the limit and
// both persist, exceeding it. Asserts that creating an openshell-user account
// (with the membership automation wired) acquires both the package-private
// per-gateway lock (name/quota invariants) AND the shared
// roleBindings.GatewayAccessLockType/GatewayAccessLockID domain (the
// membership-limit critical section) -- not just the private one.
func TestCreate_AcquiresTheSharedGatewayAccessLockDomain(t *testing.T) {
	dao := newMemoryDAO()
	kc := &fakeKeycloak{configured: true}
	locks := &countingLockFactory{}
	memberships := newFakeMemberships()
	svc := NewService(dao, fakeGateway{gateway: testGateway()}, testBindings("creator", "gateway:owner"), kc, locks, memberships)

	if _, problem := svc.Create(t.Context(), "gateway-id", "creator", CreateInput{Name: "bot", Role: RoleUser}); problem != nil {
		t.Fatalf("Create() problem = %#v", problem)
	}
	// Two nested locks: the private serviceAccountsLockType lock acquired first
	// (name/quota checks), and the shared membership-limit lock acquired after it
	// and released before it (LIFO), so it is the most recently acquired.
	if locks.acquired != 2 {
		t.Fatalf("lock acquisitions = %d, want 2 (private + shared membership-limit lock)", locks.acquired)
	}
	if locks.released != locks.acquired {
		t.Fatalf("lock releases = %d, acquisitions = %d; lock leaked", locks.released, locks.acquired)
	}
	if locks.lastType != roleBindings.GatewayAccessLockType {
		t.Fatalf("lock type = %q, want %q (roleBindings.GatewayAccessLockType, shared with gatewayAccess)", locks.lastType, roleBindings.GatewayAccessLockType)
	}
	if locks.lastID != roleBindings.GatewayAccessLockID("gateway-id") {
		t.Fatalf("lock id = %q, want %q (roleBindings.GatewayAccessLockID, shared with gatewayAccess)", locks.lastID, roleBindings.GatewayAccessLockID("gateway-id"))
	}
}

// An openshell-admin create never touches the shared membership-limit lock: it
// has no membership row and nothing to race against gatewayAccess over, so it
// acquires only the package-private per-gateway lock.
func TestCreate_AdminDoesNotAcquireTheSharedGatewayAccessLockDomain(t *testing.T) {
	dao := newMemoryDAO()
	kc := &fakeKeycloak{configured: true}
	locks := &countingLockFactory{}
	memberships := newFakeMemberships()
	svc := NewService(dao, fakeGateway{gateway: testGateway()}, testBindings("creator", "gateway:owner"), kc, locks, memberships)

	if _, problem := svc.Create(t.Context(), "gateway-id", "creator", CreateInput{Name: "admin-bot", Role: RoleAdmin}); problem != nil {
		t.Fatalf("Create() problem = %#v", problem)
	}
	if locks.acquired != 1 {
		t.Fatalf("lock acquisitions = %d, want 1 (private lock only)", locks.acquired)
	}
	if locks.lastType != serviceAccountsLockType {
		t.Fatalf("lock type = %q, want %q (package-private, not shared with gatewayAccess)", locks.lastType, serviceAccountsLockType)
	}
}

// OSG-SACC-19.08's admin-to-user downgrade creates a membership subject to the
// same limit as Create(), so it must acquire the same shared lock domain, not
// just the package-private per-gateway lock.
func TestUpdateRole_DowngradeToUserAcquiresTheSharedGatewayAccessLockDomain(t *testing.T) {
	dao := newMemoryDAO()
	now := time.Now().UTC()
	account := &OpenShellGatewayServiceAccount{
		GatewayID: "gateway-id", Name: "bot", CreatedByUserID: "creator", Status: StatusReady,
		Role: RoleAdmin, CredentialType: CredentialTypeClientSecret, KeycloakClientID: "client-id",
		KeycloakClientUUID: "client-uuid", Subject: "subject-id", ExpiresAt: now.Add(time.Hour),
	}
	dao.seed(account)
	kc := &fakeKeycloak{configured: true}
	locks := &countingLockFactory{}
	memberships := newFakeMemberships()
	svc := NewService(dao, fakeGateway{gateway: testGateway()}, testBindings("creator", "gateway:owner"), kc, locks, memberships)

	if _, _, problem := svc.UpdateRole(t.Context(), "gateway-id", account.ID, "creator", RoleUpdateInput{Role: RoleUser}); problem != nil {
		t.Fatalf("UpdateRole() problem = %#v", problem)
	}
	if locks.acquired != 2 {
		t.Fatalf("lock acquisitions = %d, want 2 (private + shared membership-limit lock)", locks.acquired)
	}
	if locks.released != locks.acquired {
		t.Fatalf("lock releases = %d, acquisitions = %d; lock leaked", locks.released, locks.acquired)
	}
	if locks.lastType != roleBindings.GatewayAccessLockType {
		t.Fatalf("lock type = %q, want %q (roleBindings.GatewayAccessLockType, shared with gatewayAccess)", locks.lastType, roleBindings.GatewayAccessLockType)
	}
	if locks.lastID != roleBindings.GatewayAccessLockID("gateway-id") {
		t.Fatalf("lock id = %q, want %q (roleBindings.GatewayAccessLockID, shared with gatewayAccess)", locks.lastID, roleBindings.GatewayAccessLockID("gateway-id"))
	}
}
