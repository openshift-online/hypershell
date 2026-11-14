package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openshift-online/hypershell/components/sandbox-resource-interceptor/internal/policy"
)

func TestRecordEvaluationCounts(t *testing.T) {
	registry := New(policy.ModeEnforce)
	registry.RecordEvaluation(policy.OutcomeAllowed, "")
	registry.RecordEvaluation(policy.OutcomePatched, "")
	registry.RecordEvaluation(policy.OutcomePatched, "")
	registry.RecordEvaluation(policy.OutcomeDenied, policy.DenialCPUBelowFloor)

	body := scrape(t, registry)
	for _, want := range []string{
		`sandbox_resource_interceptor_evaluations_total{outcome="allowed",reason=""} 1`,
		`sandbox_resource_interceptor_evaluations_total{outcome="patched",reason=""} 2`,
		`sandbox_resource_interceptor_evaluations_total{outcome="denied",reason="cpu_below_floor"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("scrape missing %q", want)
		}
	}
}

func TestModeGauge(t *testing.T) {
	registry := New(policy.ModeEnforce)
	body := scrape(t, registry)
	if !strings.Contains(body, `sandbox_resource_interceptor_mode{mode="enforce"} 1`) {
		t.Errorf("enforce mode gauge missing:\n%s", body)
	}
	if !strings.Contains(body, `sandbox_resource_interceptor_mode{mode="off"} 0`) {
		t.Errorf("off mode gauge missing:\n%s", body)
	}

	registry = New(policy.ModeOff)
	body = scrape(t, registry)
	if !strings.Contains(body, `sandbox_resource_interceptor_mode{mode="off"} 1`) {
		t.Errorf("off mode gauge missing:\n%s", body)
	}
	if !strings.Contains(body, `sandbox_resource_interceptor_mode{mode="enforce"} 0`) {
		t.Errorf("enforce mode gauge missing:\n%s", body)
	}
}

// scrape serves one /metrics request against the registry.
func scrape(t *testing.T, registry *Registry) string {
	t.Helper()
	recorder := httptest.NewRecorder()
	registry.Handler().ServeHTTP(recorder, &http.Request{Method: http.MethodGet})
	if recorder.Code != http.StatusOK {
		t.Fatalf("metrics scrape status = %d, want 200", recorder.Code)
	}
	return recorder.Body.String()
}
