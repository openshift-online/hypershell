package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-online/hypershell/tests/e2e/harness"
)

// p2_10AlertingSignal validates that an operational fault surfaces an
// observable signal (E2E-COV-04). The metric tier is unconditional in long
// mode; the deferred alert-firing tier requires an authored PrometheusRule,
// which does not exist in this repository yet, so it always t.Skipf's.
func (s *E2ESuite) p2_10AlertingSignal(t *testing.T) {
	if s.skipKubeChecks {
		t.Skip("no per-cluster kube context; skipping the alerting-signal checks")
	}
	ctx := t.Context()

	promAddress, closeForward, ok := s.portForwardPrometheus(t)
	if !ok {
		t.Skip("in-cluster Prometheus is not reachable; skipping the alerting-signal metric tier")
	}
	defer closeForward()

	const query = `sum(reconcile_errors_total{resource_kind="Gateway"})`
	baseline, err := queryPrometheusScalar(ctx, promAddress, query)
	s.Require().NoError(err, "baseline reconcile failure/outcome series query")

	// Provoke a reconcile fault: a Vault-backed gateway fails closed
	// (components/control-plane/internal/gateway/reconciler.go) when its
	// serving-CA source Secret is absent, which the Kind deploy never
	// provisions since Vault qualification is opt-in -- so this is reachable
	// unconditionally, without E2E_QUALIFY_VAULT. The reconciler classifies it
	// retryable (a later reconcile deploys once the CA appears), so it is
	// reconcile.errors, not reconcile.failed, that observably moves.
	gw := s.createGatewayWithCredentialDriver(t, "e2e-alert-"+s.runID, `{"type":"vault"}`)
	s.trackGateway(gw.ID)
	ns := s.waitGatewayNamespace(t, gw.ID)

	timeout := durationSecondsEnv("E2E_PROVISION_TIMEOUT", 180*time.Second)
	s.Require().NoError(harness.Poll(ctx, 5*time.Second, timeout, func(ctx context.Context) (bool, error) {
		value, err := queryPrometheusScalar(ctx, promAddress, query)
		if err != nil {
			return false, nil
		}
		return value > baseline, nil
	}), "reconcile failure/outcome series increments above baseline")

	s.deleteGatewayAndWaitGC(t, gw.ID, ns)

	t.Run("alert-firing", func(t *testing.T) {
		if envOrDefault("E2E_QUALIFY_ALERTING", "0") != "1" {
			t.Skip("alert-firing qualification requires E2E_QUALIFY_ALERTING=1, an authored alert rule, and a reachable Alertmanager")
		}
		t.Skip("no PrometheusRule is authored for the platform yet; alert-firing qualification has no rule to provoke")
	})
}

// portForwardPrometheus discovers the in-cluster Prometheus pod and opens a
// port-forward to its API port. ok is false when no such pod is found or the
// port-forward cannot be established, so callers can t.Skipf rather than fail.
func (s *E2ESuite) portForwardPrometheus(t *testing.T) (address string, closeForward func(), ok bool) {
	ctx := t.Context()
	pods, err := s.clients.Kube.CoreV1().Pods(s.driver.PlatformNamespace()).List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=prometheus"})
	if err != nil || len(pods.Items) == 0 {
		return "", nil, false
	}
	address, closeForward, err = s.clients.PortForwardPod(ctx, s.driver.PlatformNamespace(), pods.Items[0].Name, 9090)
	if err != nil {
		return "", nil, false
	}
	return address, closeForward, true
}

// queryPrometheusScalar issues an instant PromQL query through address (as
// returned by portForwardPrometheus) and returns the first result's value, or 0
// when the query matches no series.
func queryPrometheusScalar(ctx context.Context, address, query string) (float64, error) {
	reqURL := fmt.Sprintf("http://%s/api/v1/query?query=%s", address, url.QueryEscape(query))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return 0, fmt.Errorf("build prometheus query request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("prometheus query %q: %w", query, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0, fmt.Errorf("read prometheus query response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("prometheus query %q: status %d: %s", query, resp.StatusCode, body)
	}
	var parsed struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				Value []any `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return 0, fmt.Errorf("decode prometheus query response: %w", err)
	}
	if parsed.Status != "success" {
		return 0, fmt.Errorf("prometheus query %q did not succeed: %s", query, body)
	}
	if len(parsed.Data.Result) == 0 {
		return 0, nil
	}
	if len(parsed.Data.Result[0].Value) != 2 {
		return 0, fmt.Errorf("prometheus query %q: unexpected result value shape", query)
	}
	str, ok := parsed.Data.Result[0].Value[1].(string)
	if !ok {
		return 0, fmt.Errorf("prometheus query %q: unexpected result value type", query)
	}
	value, err := strconv.ParseFloat(str, 64)
	if err != nil {
		return 0, fmt.Errorf("parse prometheus query %q result: %w", query, err)
	}
	return value, nil
}
