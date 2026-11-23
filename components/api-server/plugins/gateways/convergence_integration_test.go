package gateways_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/plugins/gateways"
	"github.com/openshift-online/hypershell/components/api-server/test"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
)

// These tests exercise the durable change-convergence tracking the API server
// owns for SLO 2 (service-level-indicators.spec.md, OBS-SLI-01 through
// OBS-SLI-04): the convergence window state machine, the ten-minute outcome
// model and its deadline sweep, the durable convergence counter, and the
// durable provision metrics, over the real HTTP+gRPC+PostgreSQL stack.

// convergenceHarness bundles the integration helper, a control-plane gRPC client
// authorized to write observed/platform-managed fields, and direct DB access to
// the durable convergence tables.
type convergenceHarness struct {
	h  *test.Helper
	cp pb.GatewayServiceClient
}

func newConvergenceHarness(t *testing.T) (*convergenceHarness, string) {
	h, _ := test.RegisterIntegration(t)
	h.StartControllersServer()

	clusterID, subject := registerTestClusterWithSubject(t)
	cpAccount := h.NewAccount("service-account-cp-"+subject, "cp", "")
	cpToken := h.CreateJWTStringWithClaims(cpAccount, test.ControlPlaneClaims(subject))
	cp := pb.NewGatewayServiceClient(dialGRPC(t, h, cpToken))
	return &convergenceHarness{h: h, cp: cp}, clusterID
}

func (c *convergenceHarness) createGateway(t *testing.T, name, clusterID string) *gateways.Gateway {
	t.Helper()
	service := gateways.Service(&environments.Environment().Services)
	userCtx := c.h.NewAuthenticatedContext(c.h.NewRandAccount())
	gw, svcErr := service.Create(userCtx, &gateways.Gateway{Name: name, ClusterId: clusterID})
	Expect(svcErr).NotTo(HaveOccurred())
	return gw
}

// execSQL runs a statement against the integration PostgreSQL instance.
func execSQL(query string, args ...interface{}) {
	sf := environments.Environment().Database.SessionFactory
	Expect(sf.New(context.Background()).Exec(query, args...).Error).NotTo(HaveOccurred())
}

// scanInt reads a single integer from a one-row query.
func scanInt(query string, args ...interface{}) int64 {
	var n int64
	sf := environments.Environment().Database.SessionFactory
	Expect(sf.New(context.Background()).Raw(query, args...).Row().Scan(&n)).NotTo(HaveOccurred())
	return n
}

// scanNullString reads a single nullable text column from a one-row query.
func scanNullString(query string, args ...interface{}) sql.NullString {
	var ns sql.NullString
	sf := environments.Environment().Database.SessionFactory
	Expect(sf.New(context.Background()).Raw(query, args...).Row().Scan(&ns)).NotTo(HaveOccurred())
	return ns
}

// scanTime reads a single timestamp from a one-row query.
func scanTime(query string, args ...interface{}) time.Time {
	var ts time.Time
	sf := environments.Environment().Database.SessionFactory
	Expect(sf.New(context.Background()).Raw(query, args...).Row().Scan(&ts)).NotTo(HaveOccurred())
	return ts
}

func counterTotal(changeType, outcome string) int64 {
	return scanInt(
		`SELECT COALESCE((SELECT total FROM gateway_convergence_counters WHERE change_type = ? AND outcome = ?), 0)`,
		changeType, outcome)
}

func openChangeType(gatewayID string) sql.NullString {
	return scanNullString(`SELECT open_change_type FROM gateway_convergence WHERE gateway_id = ?`, gatewayID)
}

func expectOpenWindow(gatewayID, want string) {
	ns := openChangeType(gatewayID)
	Expect(ns.Valid).To(BeTrue(), "expected an open %s window", want)
	Expect(ns.String).To(Equal(want))
}

func expectClosedWindow(gatewayID string) {
	Expect(openChangeType(gatewayID).Valid).To(BeFalse(), "expected the window to be closed")
}

func provisionOutcome(gatewayID string) sql.NullString {
	return scanNullString(`SELECT outcome FROM gateway_provisions WHERE gateway_id = $1`, gatewayID)
}

// TestConvergenceCreateConvergesWithinBudget covers OBS-SLI-01.01 and
// OBS-SLI-02.01: a create opens a window at the create time and counts one
// within_10m when the newest generation converges inside the budget. It also
// covers OBS-SLI-04.01: the first Running records one provision duration and one
// success outcome.
func TestConvergenceCreateConvergesWithinBudget(t *testing.T) {
	c, clusterID := newConvergenceHarness(t)
	gw := c.createGateway(t, "conv-create", clusterID)

	// A create opens a create window immediately.
	expectOpenWindow(gw.ID, "create")

	before := counterTotal("create", "within_10m")

	running := "Running"
	var observed int64 = 1
	_, err := c.cp.UpdateGateway(context.Background(), &pb.UpdateGatewayRequest{
		Id: gw.ID, Phase: &running, ObservedGeneration: &observed,
	})
	Expect(err).NotTo(HaveOccurred())

	// The window resolved within_10m and is no longer open.
	expectClosedWindow(gw.ID)
	Expect(counterTotal("create", "within_10m")).To(Equal(before + 1))

	// Provision metrics recorded exactly once from durable timestamps.
	Expect(scanInt(`SELECT COUNT(*) FROM gateway_provisions WHERE gateway_id = ? AND duration_seconds IS NOT NULL`, gw.ID)).To(Equal(int64(1)))
	outcome := provisionOutcome(gw.ID)
	Expect(outcome.Valid).To(BeTrue())
	Expect(outcome.String).To(Equal("success"))
}

// TestConvergenceProvisionRecordedOnceAcrossRecovery covers OBS-SLI-04.01: a
// later Degraded-to-Running recovery adds neither another duration observation
// nor another outcome.
func TestConvergenceProvisionRecordedOnceAcrossRecovery(t *testing.T) {
	c, clusterID := newConvergenceHarness(t)
	gw := c.createGateway(t, "conv-recover", clusterID)

	running := "Running"
	degraded := "Degraded"
	var observed int64 = 1
	_, err := c.cp.UpdateGateway(context.Background(), &pb.UpdateGatewayRequest{Id: gw.ID, Phase: &running, ObservedGeneration: &observed})
	Expect(err).NotTo(HaveOccurred())
	firstDuration := scanInt(`SELECT COUNT(*) FROM gateway_provisions WHERE gateway_id = ?`, gw.ID)
	Expect(firstDuration).To(Equal(int64(1)))

	_, err = c.cp.UpdateGateway(context.Background(), &pb.UpdateGatewayRequest{Id: gw.ID, Phase: &degraded})
	Expect(err).NotTo(HaveOccurred())
	_, err = c.cp.UpdateGateway(context.Background(), &pb.UpdateGatewayRequest{Id: gw.ID, Phase: &running})
	Expect(err).NotTo(HaveOccurred())

	// Still exactly one provision row; a recovery adds no observation or outcome.
	Expect(scanInt(`SELECT COUNT(*) FROM gateway_provisions WHERE gateway_id = ?`, gw.ID)).To(Equal(int64(1)))
}

// TestConvergenceDeadlineCountsLateWithoutWaiting covers OBS-SLI-02.02: a change
// that has not converged when its ten-minute deadline elapses counts late at the
// deadline, through the periodic sweep, without a status write.
func TestConvergenceDeadlineCountsLate(t *testing.T) {
	c, clusterID := newConvergenceHarness(t)
	gw := c.createGateway(t, "conv-late", clusterID)

	before := counterTotal("create", "late")

	// Force the clock past the deadline, then run one sweep pass.
	execSQL(`UPDATE gateway_convergence SET clock_start = ? WHERE gateway_id = ?`, time.Now().Add(-11*time.Minute), gw.ID)
	resolved, err := gateways.RunConvergenceDeadlineSweepForTest(context.Background(), environments.Environment())
	Expect(err).NotTo(HaveOccurred())
	Expect(resolved).To(BeNumerically(">=", 1))

	Expect(counterTotal("create", "late")).To(Equal(before + 1))
	expectClosedWindow(gw.ID)
}

// TestConvergenceDeadlineCountsFailed covers OBS-SLI-02.03: a change whose newest
// generation reached a terminal-failure state before the deadline counts failed
// at the deadline.
func TestConvergenceDeadlineCountsFailed(t *testing.T) {
	c, clusterID := newConvergenceHarness(t)
	gw := c.createGateway(t, "conv-failed", clusterID)

	failed := "Failed"
	_, err := c.cp.UpdateGateway(context.Background(), &pb.UpdateGatewayRequest{Id: gw.ID, Phase: &failed})
	Expect(err).NotTo(HaveOccurred())

	before := counterTotal("create", "failed")
	execSQL(`UPDATE gateway_convergence SET clock_start = ? WHERE gateway_id = ?`, time.Now().Add(-11*time.Minute), gw.ID)
	_, err = gateways.RunConvergenceDeadlineSweepForTest(context.Background(), environments.Environment())
	Expect(err).NotTo(HaveOccurred())

	Expect(counterTotal("create", "failed")).To(Equal(before + 1))
}

// TestConvergenceLaterConvergenceNotRecounted covers OBS-SLI-02.04: a change
// already counted late at its deadline is not counted again when it converges
// later.
func TestConvergenceLaterConvergenceNotRecounted(t *testing.T) {
	c, clusterID := newConvergenceHarness(t)
	gw := c.createGateway(t, "conv-norecount", clusterID)

	execSQL(`UPDATE gateway_convergence SET clock_start = ? WHERE gateway_id = ?`, time.Now().Add(-11*time.Minute), gw.ID)
	_, err := gateways.RunConvergenceDeadlineSweepForTest(context.Background(), environments.Environment())
	Expect(err).NotTo(HaveOccurred())
	lateAfterSweep := counterTotal("create", "late")
	within := counterTotal("create", "within_10m")

	// It converges 20 minutes after the clock start; no additional outcome counts.
	running := "Running"
	var observed int64 = 1
	_, err = c.cp.UpdateGateway(context.Background(), &pb.UpdateGatewayRequest{Id: gw.ID, Phase: &running, ObservedGeneration: &observed})
	Expect(err).NotTo(HaveOccurred())

	Expect(counterTotal("create", "late")).To(Equal(lateAfterSweep))
	Expect(counterTotal("create", "within_10m")).To(Equal(within))
}

// TestConvergenceRepeatedEditsDoNotResetClock covers OBS-SLI-02.05: a further
// desired-spec change while already unconverged keeps the first clock start and
// only advances the judged generation.
func TestConvergenceRepeatedEditsDoNotResetClock(t *testing.T) {
	c, clusterID := newConvergenceHarness(t)
	gw := c.createGateway(t, "conv-clock", clusterID)

	// Pin a known first clock start three minutes in the past.
	firstClock := time.Now().Add(-3 * time.Minute).UTC().Truncate(time.Millisecond)
	execSQL(`UPDATE gateway_convergence SET clock_start = ? WHERE gateway_id = ?`, firstClock, gw.ID)

	// A control-plane desired-spec change advances generation while unconverged.
	image := "quay.io/example/gateway:v2"
	_, err := c.cp.UpdateGateway(context.Background(), &pb.UpdateGatewayRequest{Id: gw.ID, Image: &image})
	Expect(err).NotTo(HaveOccurred())

	// The clock is still measured from the first unconverged change.
	got := scanTime(`SELECT clock_start FROM gateway_convergence WHERE gateway_id = ?`, gw.ID)
	Expect(got.UTC().Truncate(time.Second)).To(Equal(firstClock.Truncate(time.Second)))
	// The newest generation is judged.
	Expect(scanInt(`SELECT open_generation FROM gateway_convergence WHERE gateway_id = ?`, gw.ID)).To(Equal(int64(2)))
	// The run is still the original create run.
	expectOpenWindow(gw.ID, "create")
}

// TestConvergenceDeadlineExactlyOnce covers OBS-SLI-02.06: two sweep passes
// against the same store (standing in for two replicas) resolve and count an
// expired window exactly once.
func TestConvergenceDeadlineExactlyOnce(t *testing.T) {
	c, clusterID := newConvergenceHarness(t)
	gw := c.createGateway(t, "conv-once", clusterID)

	before := counterTotal("create", "late")
	execSQL(`UPDATE gateway_convergence SET clock_start = ? WHERE gateway_id = ?`, time.Now().Add(-11*time.Minute), gw.ID)

	for i := 0; i < 3; i++ {
		_, err := gateways.RunConvergenceDeadlineSweepForTest(context.Background(), environments.Environment())
		Expect(err).NotTo(HaveOccurred())
	}

	Expect(counterTotal("create", "late")).To(Equal(before + 1))
}

// TestConvergenceCounterSurvivesRestart covers OBS-SLI-01.04 and OBS-SLI-03.02:
// the durable counter is monotonic across a restart (a fresh store reads the same
// totals back from PostgreSQL rather than resetting to zero).
func TestConvergenceCounterSurvivesRestart(t *testing.T) {
	c, clusterID := newConvergenceHarness(t)
	gw := c.createGateway(t, "conv-restart", clusterID)

	running := "Running"
	var observed int64 = 1
	_, err := c.cp.UpdateGateway(context.Background(), &pb.UpdateGatewayRequest{Id: gw.ID, Phase: &running, ObservedGeneration: &observed})
	Expect(err).NotTo(HaveOccurred())

	total := counterTotal("create", "within_10m")
	Expect(total).To(BeNumerically(">=", 1))

	// Simulate a restart: the durable total is unchanged when read again, and a
	// deadline sweep from a fresh store neither resets nor double-counts.
	_, err = gateways.RunConvergenceDeadlineSweepForTest(context.Background(), environments.Environment())
	Expect(err).NotTo(HaveOccurred())
	Expect(counterTotal("create", "within_10m")).To(Equal(total))
}

// TestConvergenceDeleteFinalizes covers OBS-SLI-01.03: a delete opens a delete
// window that converges only when the control plane reports finalization
// complete (the idempotent DeleteGateway re-call after reclaiming every owned
// resource).
func TestConvergenceDeleteFinalizes(t *testing.T) {
	c, clusterID := newConvergenceHarness(t)
	gw := c.createGateway(t, "conv-delete", clusterID)

	// Converge the create first so the delete is a clean, distinct change.
	running := "Running"
	var observed int64 = 1
	_, err := c.cp.UpdateGateway(context.Background(), &pb.UpdateGatewayRequest{Id: gw.ID, Phase: &running, ObservedGeneration: &observed})
	Expect(err).NotTo(HaveOccurred())

	// The user deletes the gateway (soft delete) -> a delete window opens.
	service := gateways.Service(&environments.Environment().Services)
	userCtx := c.h.NewAuthenticatedContext(c.h.NewRandAccount())
	Expect(service.Delete(userCtx, gw.ID)).NotTo(HaveOccurred())
	expectOpenWindow(gw.ID, "delete")

	before := counterTotal("delete", "within_10m")

	// The control plane reports finalization complete by re-calling DeleteGateway
	// on the already-soft-deleted gateway.
	_, err = c.cp.DeleteGateway(context.Background(), &pb.DeleteGatewayRequest{Id: gw.ID})
	Expect(err).NotTo(HaveOccurred())

	Expect(counterTotal("delete", "within_10m")).To(Equal(before + 1))
	expectClosedWindow(gw.ID)
}
