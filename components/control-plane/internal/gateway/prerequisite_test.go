package gateway

import (
	"context"
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// reconcileReport captures condition transitions emitted during a reconcile.
type reconcileReport struct{ condition, status, message string }

func collectReports(reports *[]reconcileReport) ProgressReporter {
	return func(condition, status, message string) {
		*reports = append(*reports, reconcileReport{condition, status, message})
	}
}

// When discovery confirms cert-manager is absent, the gateway reports a
// prerequisite condition identifying cert-manager and returns the confirmed-absent
// sentinel, while the controller keeps running.
// Verifies: OSG-TLS-01.02, CTRL-CORE-19.03, OSG-HLTH-07.02
func TestReconcileGatewayCertManagerConfirmedAbsent(t *testing.T) {
	disc := &fakeDiscovery{byGV: map[string]*metav1.APIResourceList{
		// Gateway API is present so the reconcile advances to the cert-manager check.
		"gateway.networking.k8s.io/v1": presentList("gateway.networking.k8s.io/v1", "GRPCRoute"),
		// cert-manager.io/v1 is absent (fakeDiscovery returns NotFound for unknown GVs).
	}}
	var reports []reconcileReport
	opts := ReconcileOpts{
		Dependencies:   NewDependencyDiscovery(disc),
		ReportProgress: collectReports(&reports),
	}
	nsConfig := NamespaceConfig{Name: "openshell-abc", Gateway: GatewayConfig{Route: RouteConfig{}}}

	err := ReconcileGateway(context.Background(), nil, nil, nil, nsConfig, opts)
	if !errors.Is(err, errCertManagerAbsent) {
		t.Fatalf("err = %v, want errCertManagerAbsent", err)
	}
	assertLastReport(t, reports, reconcileReport{ConditionEnvironmentReady, StatusFailed, certManagerAbsentMessage})
}

// When cert-manager discovery fails (transport/authorization/stale), the gateway
// is waiting/retryable: it returns an error and reports the unavailable message,
// distinct from the confirmed-absent prerequisite.
// Verifies: OSG-TLS-01.03, CTRL-CORE-19.02, OSG-HLTH-07.01
func TestReconcileGatewayCertManagerUnavailable(t *testing.T) {
	disc := &fakeDiscovery{
		byGV: map[string]*metav1.APIResourceList{
			"gateway.networking.k8s.io/v1": presentList("gateway.networking.k8s.io/v1", "GRPCRoute"),
		},
		errGV: map[string]error{
			"cert-manager.io/v1": errors.New("stale GroupVersion discovery: cert-manager.io/v1"),
		},
	}
	var reports []reconcileReport
	opts := ReconcileOpts{
		Dependencies:   NewDependencyDiscovery(disc),
		ReportProgress: collectReports(&reports),
	}
	nsConfig := NamespaceConfig{Name: "openshell-abc", Gateway: GatewayConfig{Route: RouteConfig{}}}

	err := ReconcileGateway(context.Background(), nil, nil, nil, nsConfig, opts)
	if err == nil {
		t.Fatal("want retryable error, got nil")
	}
	if errors.Is(err, errCertManagerAbsent) {
		t.Fatalf("err = %v, want unavailable (not confirmed-absent)", err)
	}
	assertLastReport(t, reports, reconcileReport{ConditionEnvironmentReady, StatusFailed, certManagerUnavailableMessage})
}

// When Gateway API discovery fails (not NotFound), the condition is the
// unavailable (retryable) message, distinct from the confirmed-absent message.
// Verifies: OSG-ROUT-02.03, CTRL-CORE-19.02, OSG-HLTH-07.01
func TestReconcileGatewayGatewayAPIUnavailable(t *testing.T) {
	disc := &fakeDiscovery{errGV: map[string]error{
		"gateway.networking.k8s.io/v1": errors.New("connection refused"),
	}}
	var reports []reconcileReport
	opts := ReconcileOpts{
		Dependencies:   NewDependencyDiscovery(disc),
		ReportProgress: collectReports(&reports),
	}
	nsConfig := NamespaceConfig{Name: "openshell-abc", Gateway: GatewayConfig{Route: RouteConfig{}}}

	err := ReconcileGateway(context.Background(), nil, nil, nil, nsConfig, opts)
	if err == nil {
		t.Fatal("want retryable error, got nil")
	}
	if errors.Is(err, errGatewayAPIMissing) {
		t.Fatalf("err = %v, want unavailable (not confirmed-absent)", err)
	}
	if !errors.Is(err, errGatewayAPIUnavailable) {
		t.Fatalf("err = %v, want errGatewayAPIUnavailable", err)
	}
	assertLastReport(t, reports, reconcileReport{ConditionEnvironmentReady, StatusFailed, gatewayAPIUnavailableMessage})
}

func assertLastReport(t *testing.T, reports []reconcileReport, want reconcileReport) {
	t.Helper()
	if len(reports) == 0 {
		t.Fatal("no condition reported")
	}
	last := reports[len(reports)-1]
	if last != want {
		t.Fatalf("last report = %+v, want %+v", last, want)
	}
	for _, r := range reports {
		if r.status == StatusComplete {
			t.Fatalf("condition %s reported Complete despite an unmet prerequisite", r.condition)
		}
	}
}
