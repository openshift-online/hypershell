package authz_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api"
	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/plugins/platformGatewayPolicies"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roles"
	"github.com/openshift-online/hypershell/components/api-server/plugins/users"
	"github.com/openshift-online/hypershell/components/api-server/test"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
)

const cpuMaxCedar = `forbid (principal, action, resource) when { context.cpuRequestMillis > 4000 };`

// userIDOf provisions the principal (one authenticated request) and returns its
// local user id.
func userIDOf(t *testing.T, client *openapi.APIClient, p principal) string {
	t.Helper()
	_, _, _ = client.DefaultAPI.ListGateways(p.rest).Execute()
	u, svcErr := users.Service(&environments.Environment().Services).GetByUsername(context.Background(), p.name)
	Expect(svcErr).NotTo(HaveOccurred(), "provision %s", p.name)
	return u.ID
}

// grantPolicyAdmin binds platform-policy:admin to p through the RoleBinding
// service as a platform:admin caller (SEC-RBAC-30 assignment path).
func grantPolicyAdmin(t *testing.T, h *test.Helper, client *openapi.APIClient, p principal) {
	t.Helper()
	admin := newPrincipal(t, h, "pgp-granter-"+short(), realmRoles("platform:admin"))
	adminID := userIDOf(t, client, admin)
	targetID := userIDOf(t, client, p)
	role, svcErr := roles.Service(&environments.Environment().Services).GetByName(context.Background(), roles.RolePlatformPolicyAdmin)
	Expect(svcErr).NotTo(HaveOccurred(), "platform-policy:admin must be seeded")
	ctx := context.WithValue(context.Background(), rbac.ContextUserIDKey, adminID)
	_, svcErr = roleBindings.Service(&environments.Environment().Services).Create(ctx, &roleBindings.RoleBinding{
		RoleID: role.ID, Scope: roleBindings.ScopeGlobal, UserID: &targetID,
	})
	Expect(svcErr).NotTo(HaveOccurred(), "grant platform-policy:admin")
}

func grantGatewayRole(t *testing.T, client *openapi.APIClient, p principal, gatewayID, role string) {
	t.Helper()
	uid := userIDOf(t, client, p)
	_, svcErr := roleBindings.Service(&environments.Environment().Services).SetGatewayRole(context.Background(), uid, gatewayID, role)
	Expect(svcErr).NotTo(HaveOccurred(), "grant %s", role)
}

func createPolicy(t *testing.T, client *openapi.APIClient, p principal, name, cedarText string) (*openapi.PlatformGatewayPolicy, *http.Response, error) {
	t.Helper()
	req := openapi.NewPlatformGatewayPolicy(name, cedarText)
	req.UserMessage = openapi.PtrString("Keep sandboxes small.")
	return client.DefaultAPI.CreatePlatformGatewayPolicy(p.rest).PlatformGatewayPolicy(*req).Execute()
}

// Verifies: SEC-RBAC-30.01, SEC-RBAC-30.02, SEC-RBAC-30.04, PLAT-PGP-01.01, PLAT-PGP-01.02, PLAT-DATA-28.01, PLAT-PGP-06.02, PLAT-DATA-29.01, PLAT-DATA-29.02
func TestPlatformPolicyAdminAuthorsPoliciesAndToggle(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	spoke := registerSpoke(t, h, client)
	owner := newPrincipal(t, h, "pgp-owner-"+short(), realmRoles("gateway:creator"))
	gw := createGateway(t, client, owner, spoke.clusterID)
	policyAdmin := newPrincipal(t, h, "pgp-admin-"+short(), nil)
	grantPolicyAdmin(t, h, client, policyAdmin)

	// The gateway owner of a targeted gateway is not a policy author (403).
	_, resp, err := createPolicy(t, client, owner, "owner-attempt", cpuMaxCedar)
	Expect(err).To(HaveOccurred())
	Expect(statusOf(resp)).To(Equal(http.StatusForbidden))
	_, resp, err = client.DefaultAPI.UpdatePlatformPolicySettings(owner.rest).
		PlatformPolicySettingsPatchRequest(*openapi.NewPlatformPolicySettingsPatchRequest("enforce")).Execute()
	Expect(err).To(HaveOccurred())
	Expect(statusOf(resp)).To(Equal(http.StatusForbidden))

	// The default toggle is audit (PLAT-DATA-29.01).
	settings, _, err := client.DefaultAPI.GetPlatformPolicySettings(policyAdmin.rest).Execute()
	Expect(err).NotTo(HaveOccurred(), test.APIErrorReason(err))
	Expect(settings.GetMode()).To(Equal("audit"))
	Expect(settings.Defaults.GetCpuRequest()).To(Equal("500m"))

	// A create asking for enforce is stored as audit (PLAT-PGP-01.02).
	req := openapi.NewPlatformGatewayPolicy("cpu-max", cpuMaxCedar)
	req.EnforcementMode = openapi.PtrString("enforce")
	req.DocumentationUrl = openapi.PtrString("https://docs.example.com/cpu")
	req.Target = &openapi.PlatformGatewayPolicyTarget{All: openapi.PtrBool(true)}
	created, resp, err := client.DefaultAPI.CreatePlatformGatewayPolicy(policyAdmin.rest).PlatformGatewayPolicy(*req).Execute()
	Expect(err).NotTo(HaveOccurred(), "HTTP %d %s", statusOf(resp), test.APIErrorReason(err))
	Expect(created.GetEnforcementMode()).To(Equal("audit"))
	Expect(created.GetEffectiveMode()).To(Equal("audit"))
	Expect(created.GetCedar()).To(Equal(cpuMaxCedar))
	Expect(created.GetDocumentationUrl()).To(Equal("https://docs.example.com/cpu"))
	Expect(created.GetRendered()).To(HaveLen(1))
	Expect(created.GetRendered()[0].GetSummary()).To(ConsistOf("CPU request: at most 4"))

	// The owner cannot change the mode or delete; a direct id read is masked.
	_, resp, err = client.DefaultAPI.UpdatePlatformGatewayPolicy(owner.rest, created.GetId()).
		PlatformGatewayPolicyPatchRequest(openapi.PlatformGatewayPolicyPatchRequest{EnforcementMode: openapi.PtrString("enforce")}).Execute()
	Expect(err).To(HaveOccurred())
	Expect(statusOf(resp)).To(Equal(http.StatusForbidden))
	resp, err = client.DefaultAPI.DeletePlatformGatewayPolicy(owner.rest, created.GetId()).Execute()
	Expect(err).To(HaveOccurred())
	Expect(statusOf(resp)).To(Equal(http.StatusForbidden))
	_, resp, err = client.DefaultAPI.GetPlatformGatewayPolicy(owner.rest, created.GetId()).Execute()
	Expect(err).To(HaveOccurred())
	Expect(statusOf(resp)).To(Equal(http.StatusNotFound))

	// The policy admin raises the mode and moves the toggle.
	updated, _, err := client.DefaultAPI.UpdatePlatformGatewayPolicy(policyAdmin.rest, created.GetId()).
		PlatformGatewayPolicyPatchRequest(openapi.PlatformGatewayPolicyPatchRequest{EnforcementMode: openapi.PtrString("enforce")}).Execute()
	Expect(err).NotTo(HaveOccurred(), test.APIErrorReason(err))
	Expect(updated.GetEnforcementMode()).To(Equal("enforce"))
	settings, _, err = client.DefaultAPI.UpdatePlatformPolicySettings(policyAdmin.rest).
		PlatformPolicySettingsPatchRequest(*openapi.NewPlatformPolicySettingsPatchRequest("enforce")).Execute()
	Expect(err).NotTo(HaveOccurred(), test.APIErrorReason(err))
	Expect(settings.GetMode()).To(Equal("enforce"))
	Expect(settings.GetUpdatedBy()).To(Equal(policyAdmin.name))

	// The role grants nothing else (SEC-RBAC-30.04).
	_, err = pb.NewGatewayServiceClient(policyAdmin.conn).CreateGateway(context.Background(), &pb.CreateGatewayRequest{Name: "pgp-gw-" + short(), ClusterId: spoke.clusterID})
	Expect(grpcCode(err)).To(Equal(codes.PermissionDenied))
	_, resp, err = client.DefaultAPI.GetGateway(policyAdmin.rest, *gw.Id).Execute()
	Expect(err).To(HaveOccurred())
	Expect(statusOf(resp)).To(Equal(http.StatusNotFound))
	_, resp, err = client.DefaultAPI.CreateSandboxTemplate(policyAdmin.rest).
		SandboxTemplate(*openapi.NewSandboxTemplate("st-"+short(), "quay.io/x/y:1")).Execute()
	Expect(err).To(HaveOccurred())
	Expect(statusOf(resp)).To(Equal(http.StatusForbidden))

	// Delete works for a regular policy.
	resp, err = client.DefaultAPI.DeletePlatformGatewayPolicy(policyAdmin.rest, created.GetId()).Execute()
	Expect(err).NotTo(HaveOccurred(), test.APIErrorReason(err))
	Expect(statusOf(resp)).To(Equal(http.StatusNoContent))
}

// Verifies: PLAT-PGP-04.01, PLAT-PGP-04.02, PLAT-PGP-05.01, PLAT-PGP-02
func TestPlatformPolicyWriteTimeValidation(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	policyAdmin := newPrincipal(t, h, "pgp-admin-"+short(), realmRoles("platform:admin"))

	cases := map[string]struct {
		cedar string
		want  string
	}{
		"permit":            {`permit (principal, action, resource);`, "permit is not allowed"},
		"unknown attribute": {`forbid (principal, action, resource) when { context.cpuRequest > 4 };`, "context.cpuRequestMillis"},
		"forbids defaults":  {`forbid (principal, action, resource) when { context.cpuRequestMillis < 600 };`, "forbids the default sandbox request"},
		"annotation":        {`@id("x") forbid (principal, action, resource) when { context.gpuCount > 4 };`, "annotations are not allowed"},
	}
	for name, tc := range cases {
		_, resp, err := createPolicy(t, client, policyAdmin, "v-"+short(), tc.cedar)
		Expect(err).To(HaveOccurred(), name)
		Expect(statusOf(resp)).To(Equal(http.StatusBadRequest), name)
		Expect(test.APIErrorReason(err)).To(ContainSubstring(tc.want), name)
	}

	// The editor dry run reports the same diagnostics with a line and a fix
	// and evaluates a sample request with enforcement semantics.
	eval, _, err := client.DefaultAPI.EvaluatePlatformGatewayPolicy(policyAdmin.rest).
		PlatformGatewayPolicyEvaluationRequest(openapi.PlatformGatewayPolicyEvaluationRequest{
			Name:        openapi.PtrString("cpu-max"),
			Cedar:       cpuMaxCedar,
			UserMessage: openapi.PtrString("Keep sandboxes small."),
			Sample:      &openapi.PolicySampleRequest{CpuRequest: openapi.PtrString("8")},
		}).Execute()
	Expect(err).NotTo(HaveOccurred(), test.APIErrorReason(err))
	Expect(eval.GetValid()).To(BeTrue())
	Expect(eval.GetSampleAllowed()).To(BeFalse())
	Expect(eval.GetDenialText()).To(ContainSubstring("cpu-max: Keep sandboxes small."))
	Expect(eval.GetDenialText()).To(ContainSubstring("CPU request is 8; allowed: at most 4"))

	eval, _, err = client.DefaultAPI.EvaluatePlatformGatewayPolicy(policyAdmin.rest).
		PlatformGatewayPolicyEvaluationRequest(openapi.PlatformGatewayPolicyEvaluationRequest{
			Cedar: `forbid (principal, action, resource) when { context.cpuRequestMillis < 600 };`,
		}).Execute()
	Expect(err).NotTo(HaveOccurred(), test.APIErrorReason(err))
	Expect(eval.GetValid()).To(BeFalse())
	Expect(eval.GetForbidsDefaults()).To(BeTrue())
}

// Verifies: PLAT-PGP-20.02, PLAT-DATA-28.02
func TestCanaryPolicyIsSeededAndUndeletable(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	policyAdmin := newPrincipal(t, h, "pgp-admin-"+short(), nil)
	grantPolicyAdmin(t, h, client, policyAdmin)

	list, _, err := client.DefaultAPI.ListPlatformGatewayPolicies(policyAdmin.rest).
		Search("name = 'e2e-enforcement-canary'").Execute()
	Expect(err).NotTo(HaveOccurred(), test.APIErrorReason(err))
	Expect(list.GetItems()).To(HaveLen(1))
	canary := list.GetItems()[0]
	Expect(canary.GetBuiltIn()).To(BeTrue())
	Expect(canary.GetEnforcementMode()).To(Equal("enforce"))
	Expect(canary.Target.GetAll()).To(BeTrue())
	Expect(canary.GetUserMessage()).To(ContainSubstring("e2epol-"))

	resp, err := client.DefaultAPI.DeletePlatformGatewayPolicy(policyAdmin.rest, canary.GetId()).Execute()
	Expect(err).To(HaveOccurred())
	Expect(statusOf(resp)).To(Equal(http.StatusConflict))
	_, resp, err = client.DefaultAPI.UpdatePlatformGatewayPolicy(policyAdmin.rest, canary.GetId()).
		PlatformGatewayPolicyPatchRequest(openapi.PlatformGatewayPolicyPatchRequest{EnforcementMode: openapi.PtrString("audit")}).Execute()
	Expect(err).To(HaveOccurred())
	Expect(statusOf(resp)).To(Equal(http.StatusConflict))
}

func reportFinding(t *testing.T, s spoke, gatewayID, id, policy, outcome string, at time.Time) (*pb.ReportPolicyAuditFindingsResponse, error) {
	t.Helper()
	return pb.NewPolicyAuditFindingServiceClient(s.conn).ReportPolicyAuditFindings(context.Background(), &pb.ReportPolicyAuditFindingsRequest{
		GatewayId: gatewayID,
		Findings: []*pb.PolicyAuditFinding{{
			Id:               id,
			OccurredAt:       timestamppb.New(at),
			GatewayId:        gatewayID,
			PolicyNames:      []string{policy},
			BundleRevision:   "rev-1",
			PrincipalSubject: "alice",
			Operation:        "CreateSandbox",
			Outcome:          outcome,
			RequestSummary:   map[string]int64{"cpuRequestMillis": 8000},
		}},
	})
}

// Verifies: PLAT-PGP-19.01, PLAT-PGP-19.02, SEC-RBAC-31.01, SEC-RBAC-31.02, SEC-RBAC-31.03, PLAT-PGP-15.01, PLAT-DATA-30.01, SEC-RBAC-32.02, CTRL-CORE-22.03
func TestEffectivePoliciesAndFindingsAreGatewayScoped(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	spoke := registerSpoke(t, h, client)
	other := registerSpoke(t, h, client)
	owner := newPrincipal(t, h, "pgp-owner-"+short(), realmRoles("gateway:creator"))
	gwG := createGateway(t, client, owner, spoke.clusterID)
	gwH := createGateway(t, client, owner, spoke.clusterID)
	viewer := newPrincipal(t, h, "pgp-viewer-"+short(), nil)
	grantGatewayRole(t, client, viewer, *gwG.Id, "gateway:viewer")
	policyAdmin := newPrincipal(t, h, "pgp-admin-"+short(), nil)
	grantPolicyAdmin(t, h, client, policyAdmin)

	req := openapi.NewPlatformGatewayPolicy("cpu-max", cpuMaxCedar)
	req.UserMessage = openapi.PtrString("Keep sandboxes small.")
	req.Target = &openapi.PlatformGatewayPolicyTarget{All: openapi.PtrBool(true), Exclude: []string{*gwH.Id}}
	policy, _, err := client.DefaultAPI.CreatePlatformGatewayPolicy(policyAdmin.rest).PlatformGatewayPolicy(*req).Execute()
	Expect(err).NotTo(HaveOccurred(), test.APIErrorReason(err))

	// The registered control plane forwards findings; a retry is idempotent.
	now := time.Now().UTC()
	id := "evt-" + strings.ToLower(api.NewID())
	res, err := reportFinding(t, spoke, *gwG.Id, id, "cpu-max", "would_reject", now)
	Expect(err).NotTo(HaveOccurred())
	Expect(res.GetStored()).To(Equal(int32(1)))
	res, err = reportFinding(t, spoke, *gwG.Id, id, "cpu-max", "would_reject", now)
	Expect(err).NotTo(HaveOccurred())
	Expect(res.GetStored()).To(Equal(int32(0)), "a retried event id must not store a second row")
	_, err = reportFinding(t, spoke, *gwH.Id, "evt-"+strings.ToLower(api.NewID()), "cpu-max", "would_reject", now)
	Expect(err).NotTo(HaveOccurred())

	// A control plane cannot report for a gateway on another cluster, and a user
	// cannot report at all (SEC-RBAC-32).
	_, err = reportFinding(t, other, *gwG.Id, "evt-"+strings.ToLower(api.NewID()), "cpu-max", "would_reject", now)
	Expect(grpcCode(err)).To(Equal(codes.PermissionDenied))
	_, err = pb.NewPolicyAuditFindingServiceClient(owner.conn).ReportPolicyAuditFindings(context.Background(), &pb.ReportPolicyAuditFindingsRequest{GatewayId: *gwG.Id})
	Expect(grpcCode(err)).To(Equal(codes.PermissionDenied))

	// Status write-back is own-cluster only and status-only (CTRL-CORE-22.03).
	cp := pb.NewPlatformGatewayPolicyServiceClient(spoke.conn)
	_, err = cp.UpdatePlatformGatewayPolicyStatus(context.Background(), &pb.UpdatePlatformGatewayPolicyStatusRequest{
		Id: policy.GetId(),
		GatewayStatuses: []*pb.PlatformGatewayPolicyGatewayStatus{{
			GatewayId: *gwG.Id, Condition: "Applied", Revision: "rev-1", AppliedRevision: "rev-1", EffectiveMode: "audit",
		}},
	})
	Expect(err).NotTo(HaveOccurred())
	_, err = pb.NewPlatformGatewayPolicyServiceClient(other.conn).UpdatePlatformGatewayPolicyStatus(context.Background(), &pb.UpdatePlatformGatewayPolicyStatusRequest{
		Id:              policy.GetId(),
		GatewayStatuses: []*pb.PlatformGatewayPolicyGatewayStatus{{GatewayId: *gwG.Id, Condition: "Pending"}},
	})
	Expect(grpcCode(err)).To(Equal(codes.PermissionDenied))

	// The viewer of G reads rules, defaults, mode, and G's findings, no targets.
	eff, _, err := client.DefaultAPI.GetGatewayEffectivePolicies(viewer.rest, *gwG.Id).Execute()
	Expect(err).NotTo(HaveOccurred(), test.APIErrorReason(err))
	Expect(eff.GetGlobalMode()).To(Equal("audit"))
	Expect(eff.Defaults.GetMemoryLimit()).To(Equal("4Gi"))
	names := []string{}
	for _, p := range eff.GetPolicies() {
		names = append(names, p.GetName())
		Expect(p.Target).To(BeNil(), "targets must be hidden from gateway users")
		if p.GetName() == "cpu-max" {
			Expect(p.GetCondition()).To(Equal("Applied"))
			Expect(p.GetEffectiveMode()).To(Equal("audit"))
		}
		if p.GetName() == "e2e-enforcement-canary" {
			Expect(p.GetEffectiveMode()).To(Equal("enforce"))
			Expect(p.GetEffectiveModeReason()).To(Equal("policy_enforced"))
		}
	}
	Expect(names).To(ContainElements("cpu-max", "e2e-enforcement-canary"))
	Expect(eff.GetFindings()).To(HaveLen(1))
	Expect(eff.GetFindings()[0].GetGatewayId()).To(Equal(*gwG.Id))
	Expect(eff.FindingsLast24h.GetWouldReject()).To(Equal(int32(1)))

	// The viewer of G has no binding on H: masked as 404.
	_, resp, err := client.DefaultAPI.GetGatewayEffectivePolicies(viewer.rest, *gwH.Id).Execute()
	Expect(err).To(HaveOccurred())
	Expect(statusOf(resp)).To(Equal(http.StatusNotFound))

	// The excluded gateway H does not list the policy.
	effH, _, err := client.DefaultAPI.GetGatewayEffectivePolicies(owner.rest, *gwH.Id).Execute()
	Expect(err).NotTo(HaveOccurred(), test.APIErrorReason(err))
	for _, p := range effH.GetPolicies() {
		Expect(p.GetName()).NotTo(Equal("cpu-max"))
	}

	// The policy admin sees targets on the same read (SEC-RBAC-31.02).
	effAdmin, _, err := client.DefaultAPI.GetGatewayEffectivePolicies(policyAdmin.rest, *gwG.Id).Execute()
	Expect(err).NotTo(HaveOccurred(), test.APIErrorReason(err))
	for _, p := range effAdmin.GetPolicies() {
		if p.GetName() == "cpu-max" {
			Expect(p.Target.GetExclude()).To(ConsistOf(*gwH.Id))
		}
	}

	// Policy status carries the rollout and the 24-hour counts.
	got, _, err := client.DefaultAPI.GetPlatformGatewayPolicy(policyAdmin.rest, policy.GetId()).Execute()
	Expect(err).NotTo(HaveOccurred(), test.APIErrorReason(err))
	Expect(got.Status.Rollout.GetApplied()).To(Equal(int32(1)))
	Expect(got.Status.FindingsLast24h.GetWouldReject()).To(Equal(int32(2)))
	findings, _, err := client.DefaultAPI.ListPlatformGatewayPolicyFindings(policyAdmin.rest, policy.GetId()).GatewayId(*gwG.Id).Execute()
	Expect(err).NotTo(HaveOccurred(), test.APIErrorReason(err))
	Expect(findings.GetItems()).To(HaveLen(1))
}

// Verifies: PLAT-DATA-30.02, PLAT-PGP-15
func TestPolicyFindingsArePrunedAfterRetention(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	spoke := registerSpoke(t, h, client)
	owner := newPrincipal(t, h, "pgp-owner-"+short(), realmRoles("gateway:creator"))
	gw := createGateway(t, client, owner, spoke.clusterID)

	now := time.Now().UTC()
	_, err := reportFinding(t, spoke, *gw.Id, "evt-old-"+short(), "p", "would_reject", now.Add(-31*24*time.Hour))
	Expect(err).NotTo(HaveOccurred())
	_, err = reportFinding(t, spoke, *gw.Id, "evt-new-"+short(), "p", "would_add_defaults", now)
	Expect(err).NotTo(HaveOccurred())

	svc := platformGatewayPolicies.Service(&environments.Environment().Services)
	n, svcErr := svc.PruneFindings(context.Background(), now)
	Expect(svcErr).NotTo(HaveOccurred())
	Expect(n).To(Equal(int64(1)))
	counts, svcErr := svc.CountFindings(context.Background(), platformGatewayPolicies.FindingQuery{GatewayID: *gw.Id, Since: now.Add(-24 * time.Hour)})
	Expect(svcErr).NotTo(HaveOccurred())
	Expect(counts.WouldReject).To(Equal(int32(0)))
	Expect(counts.WouldAddDefaults).To(Equal(int32(1)))
}

// rawSettingsPatch sends a PATCH body the generated client cannot express (it
// only models mode), to prove the read-only fields are not settable.
func rawSettingsPatch(t *testing.T, client *openapi.APIClient, p principal, body string) int {
	t.Helper()
	base := client.GetConfig().Servers[0].URL
	req, err := http.NewRequest(http.MethodPatch, base+"/api/hypershell/ext/platform_policy_settings", strings.NewReader(body))
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set("Authorization", "Bearer "+p.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	_ = resp.Body.Close()
	return resp.StatusCode
}

func fleetDefaults(cpuRequest string) platformGatewayPolicies.FleetDefaults {
	return platformGatewayPolicies.FleetDefaults{CPURequest: cpuRequest, CPULimit: "2", MemoryRequest: "1Gi", MemoryLimit: "4Gi"}
}

// Verifies: PLAT-PGP-08.02, PLAT-PGP-08.03, PLAT-DATA-29.03, PLAT-PGP-04.02
func TestFleetDefaultsAcceptedOrRefusedAtTheAPIServer(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	spoke := registerSpoke(t, h, client)
	policyAdmin := newPrincipal(t, h, "pgp-admin-"+short(), nil)
	grantPolicyAdmin(t, h, client, policyAdmin)
	svc := platformGatewayPolicies.Service(&environments.Environment().Services)
	ctx := context.Background()

	// A fresh database accepts the configured defaults (500m) and serves them
	// as read-only settings fields, over REST and over the gRPC settings the
	// control planes watch.
	settings, _, err := client.DefaultAPI.GetPlatformPolicySettings(policyAdmin.rest).Execute()
	Expect(err).NotTo(HaveOccurred(), test.APIErrorReason(err))
	Expect(settings.Defaults.GetCpuRequest()).To(Equal("500m"))
	Expect(settings.Defaults.GetCpuRequestMillis()).To(Equal(int64(500)))
	Expect(settings.Defaults.GetMemoryLimitBytes()).To(Equal(int64(4 << 30)))
	Expect(settings.DefaultsAcceptedAt).NotTo(BeNil())
	Expect(settings.GetDefaultsRefusal()).To(BeEmpty())

	// An operator raises the default CPU request to 700m; no policy forbids it,
	// so it is accepted and persisted.
	st, svcErr := svc.ReconcileDefaults(ctx, fleetDefaults("700m"))
	Expect(svcErr).NotTo(HaveOccurred())
	Expect(st.DefaultCPURequest).To(Equal("700m"))
	Expect(st.DefaultsRefusal).To(BeEmpty())

	// Write-time validation now checks the accepted 700m (PLAT-PGP-04 rule 3):
	// a policy forbidding a CPU request below 600m is valid and is enforced.
	minCPU, resp, err := createPolicy(t, client, policyAdmin, "min-cpu", `forbid (principal, action, resource) when { context.cpuRequestMillis < 600 };`)
	Expect(err).NotTo(HaveOccurred(), "HTTP %d %s", statusOf(resp), test.APIErrorReason(err))
	_, _, err = client.DefaultAPI.UpdatePlatformGatewayPolicy(policyAdmin.rest, minCPU.GetId()).
		PlatformGatewayPolicyPatchRequest(openapi.PlatformGatewayPolicyPatchRequest{EnforcementMode: openapi.PtrString("enforce")}).Execute()
	Expect(err).NotTo(HaveOccurred(), test.APIErrorReason(err))

	// The operator lowers the default to 500m and the api-server restarts: the
	// enforced policy forbids the new default request, so the change is refused
	// and 700m keeps being served with the reason (PLAT-PGP-08.02).
	st, svcErr = svc.ReconcileDefaults(ctx, fleetDefaults("500m"))
	Expect(svcErr).NotTo(HaveOccurred())
	Expect(st.DefaultCPURequest).To(Equal("700m"))
	Expect(st.DefaultsRefusal).To(ContainSubstring("min-cpu"))
	settings, _, err = client.DefaultAPI.GetPlatformPolicySettings(policyAdmin.rest).Execute()
	Expect(err).NotTo(HaveOccurred(), test.APIErrorReason(err))
	Expect(settings.Defaults.GetCpuRequest()).To(Equal("700m"))
	Expect(settings.GetDefaultsRefusal()).To(ContainSubstring("min-cpu"))
	got, err := pb.NewPlatformPolicySettingsServiceClient(spoke.conn).GetPlatformPolicySettings(ctx, &pb.GetPlatformPolicySettingsRequest{})
	Expect(err).NotTo(HaveOccurred())
	Expect(got.GetSettings().GetDefaults().GetCpuRequest()).To(Equal("700m"))
	Expect(got.GetSettings().GetDefaults().GetCpuRequestMillis()).To(Equal(int64(700)))
	Expect(got.GetSettings().GetDefaultsRefusal()).To(ContainSubstring("min-cpu"))

	// The defaults are read-only: a PATCH that also sends defaults is rejected
	// and changes nothing (PLAT-DATA-29.03).
	code := rawSettingsPatch(t, client, policyAdmin, `{"mode":"audit","defaults":{"cpu_request":"100m"},"defaults_refusal":""}`)
	Expect(code).To(Equal(http.StatusBadRequest))
	settings, _, err = client.DefaultAPI.GetPlatformPolicySettings(policyAdmin.rest).Execute()
	Expect(err).NotTo(HaveOccurred(), test.APIErrorReason(err))
	Expect(settings.Defaults.GetCpuRequest()).To(Equal("700m"))
	Expect(settings.GetDefaultsRefusal()).To(ContainSubstring("min-cpu"))

	// Write-time validation still uses the accepted 700m, not the refused 500m.
	_, _, err = createPolicy(t, client, policyAdmin, "min-cpu-650", `forbid (principal, action, resource) when { context.cpuRequestMillis < 650 };`)
	Expect(err).NotTo(HaveOccurred(), test.APIErrorReason(err))

	// With the forbidding policy back in audit, the next start accepts 500m and
	// clears the refusal. min-cpu-650 is audit-only, so it does not block.
	_, _, err = client.DefaultAPI.UpdatePlatformGatewayPolicy(policyAdmin.rest, minCPU.GetId()).
		PlatformGatewayPolicyPatchRequest(openapi.PlatformGatewayPolicyPatchRequest{EnforcementMode: openapi.PtrString("audit")}).Execute()
	Expect(err).NotTo(HaveOccurred(), test.APIErrorReason(err))
	st, svcErr = svc.ReconcileDefaults(ctx, fleetDefaults("500m"))
	Expect(svcErr).NotTo(HaveOccurred())
	Expect(st.DefaultCPURequest).To(Equal("500m"))
	Expect(st.DefaultsRefusal).To(BeEmpty())
	got, err = pb.NewPlatformPolicySettingsServiceClient(spoke.conn).GetPlatformPolicySettings(ctx, &pb.GetPlatformPolicySettingsRequest{})
	Expect(err).NotTo(HaveOccurred())
	Expect(got.GetSettings().GetDefaults().GetCpuRequest()).To(Equal("500m"))
	Expect(got.GetSettings().GetDefaultsRefusal()).To(BeEmpty())

	// An invalid configured quantity is rejected, never accepted.
	_, svcErr = svc.ReconcileDefaults(ctx, fleetDefaults("lots"))
	Expect(svcErr).To(HaveOccurred())
}

// Verifies: PLAT-PGP-21.01, PLAT-PGP-21.02
func TestPolicyDryRunAndFindingsAccess(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	spoke := registerSpoke(t, h, client)
	owner := newPrincipal(t, h, "pgp-owner-"+short(), realmRoles("gateway:creator"))
	gwG := createGateway(t, client, owner, spoke.clusterID)
	gwH := createGateway(t, client, owner, spoke.clusterID)
	policyAdmin := newPrincipal(t, h, "pgp-admin-"+short(), nil)
	grantPolicyAdmin(t, h, client, policyAdmin)
	reader := newPrincipal(t, h, "pgp-reader-"+short(), realmRoles(roles.RoleFleetReader))
	svc := platformGatewayPolicies.Service(&environments.Environment().Services)
	ctx := context.Background()

	// PLAT-PGP-21.01: the dry run returns diagnostics, the rendered summary,
	// the defaults check, and the decision with its denial text, and saves
	// nothing.
	before, svcErr := svc.All(ctx)
	Expect(svcErr).NotTo(HaveOccurred())
	eval, resp, err := client.DefaultAPI.EvaluatePlatformGatewayPolicy(policyAdmin.rest).
		PlatformGatewayPolicyEvaluationRequest(openapi.PlatformGatewayPolicyEvaluationRequest{
			Name:        openapi.PtrString("draft-cpu-max"),
			Cedar:       cpuMaxCedar,
			UserMessage: openapi.PtrString("Keep sandboxes small."),
			Sample:      &openapi.PolicySampleRequest{CpuRequest: openapi.PtrString("8")},
		}).Execute()
	Expect(err).NotTo(HaveOccurred(), "HTTP %d %s", statusOf(resp), test.APIErrorReason(err))
	Expect(eval.GetValid()).To(BeTrue())
	Expect(eval.GetDiagnostics()).To(BeEmpty())
	Expect(eval.GetRendered()).To(HaveLen(1))
	Expect(eval.GetRendered()[0].GetSummary()).To(ConsistOf("CPU request: at most 4"))
	Expect(eval.GetForbidsDefaults()).To(BeFalse())
	Expect(eval.Defaults.GetCpuRequest()).To(Equal("500m"))
	Expect(eval.GetSampleAllowed()).To(BeFalse())
	Expect(eval.GetDenialText()).To(ContainSubstring("draft-cpu-max: Keep sandboxes small."))

	invalid, _, err := client.DefaultAPI.EvaluatePlatformGatewayPolicy(policyAdmin.rest).
		PlatformGatewayPolicyEvaluationRequest(openapi.PlatformGatewayPolicyEvaluationRequest{
			Cedar: "forbid (principal, action, resource)\nwhen { context.cpuRequest > 4 };",
		}).Execute()
	Expect(err).NotTo(HaveOccurred(), test.APIErrorReason(err))
	Expect(invalid.GetValid()).To(BeFalse())
	Expect(invalid.GetDiagnostics()).NotTo(BeEmpty())
	Expect(invalid.GetDiagnostics()[0].GetLine()).To(BeNumerically(">", 0))

	after, svcErr := svc.All(ctx)
	Expect(svcErr).NotTo(HaveOccurred())
	Expect(after).To(HaveLen(len(before)), "the dry run must persist nothing")
	for _, p := range after {
		Expect(p.Name).NotTo(Equal("draft-cpu-max"))
	}

	// The dry run is for policy authors only.
	_, resp, err = client.DefaultAPI.EvaluatePlatformGatewayPolicy(owner.rest).
		PlatformGatewayPolicyEvaluationRequest(openapi.PlatformGatewayPolicyEvaluationRequest{Cedar: cpuMaxCedar}).Execute()
	Expect(err).To(HaveOccurred())
	Expect(statusOf(resp)).To(Equal(http.StatusForbidden))

	// PLAT-PGP-21.02: policy P with findings on two gateways.
	policy, _, err := createPolicy(t, client, policyAdmin, "cpu-max", cpuMaxCedar)
	Expect(err).NotTo(HaveOccurred(), test.APIErrorReason(err))
	now := time.Now().UTC()
	for _, gw := range []string{*gwG.Id, *gwH.Id} {
		_, err = reportFinding(t, spoke, gw, "evt-"+strings.ToLower(api.NewID()), "cpu-max", "would_reject", now)
		Expect(err).NotTo(HaveOccurred())
	}
	_, err = reportFinding(t, spoke, *gwG.Id, "evt-"+strings.ToLower(api.NewID()), "cpu-max", "would_add_defaults", now)
	Expect(err).NotTo(HaveOccurred())

	list, _, err := client.DefaultAPI.ListPlatformGatewayPolicyFindings(policyAdmin.rest, policy.GetId()).
		Outcome("would_reject").Size(1).Execute()
	Expect(err).NotTo(HaveOccurred(), test.APIErrorReason(err))
	Expect(list.GetTotal()).To(Equal(int32(2)))
	Expect(list.GetItems()).To(HaveLen(1), "the findings are paginated")
	page2, _, err := client.DefaultAPI.ListPlatformGatewayPolicyFindings(policyAdmin.rest, policy.GetId()).
		Outcome("would_reject").Size(1).Page(2).Execute()
	Expect(err).NotTo(HaveOccurred(), test.APIErrorReason(err))
	Expect(page2.GetItems()).To(HaveLen(1))
	Expect([]string{list.GetItems()[0].GetGatewayId(), page2.GetItems()[0].GetGatewayId()}).To(ConsistOf(*gwG.Id, *gwH.Id))
	for _, f := range append(list.GetItems(), page2.GetItems()...) {
		Expect(f.GetOutcome()).To(Equal("would_reject"))
	}

	// fleet:reader reads the same findings (SEC-RBAC-31).
	readerList, _, err := client.DefaultAPI.ListPlatformGatewayPolicyFindings(reader.rest, policy.GetId()).
		Outcome("would_reject").Execute()
	Expect(err).NotTo(HaveOccurred(), test.APIErrorReason(err))
	Expect(readerList.GetTotal()).To(Equal(int32(2)))

	// The owner of both gateways is refused the per-policy findings path.
	_, resp, err = client.DefaultAPI.ListPlatformGatewayPolicyFindings(owner.rest, policy.GetId()).
		Outcome("would_reject").Execute()
	Expect(err).To(HaveOccurred())
	Expect(statusOf(resp)).To(BeElementOf(http.StatusForbidden, http.StatusNotFound))
}
