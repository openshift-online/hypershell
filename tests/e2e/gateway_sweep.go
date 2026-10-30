package e2e

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	sdktypes "github.com/openshift-online/hypershell/components/sdk-go/types"
)

// e2eGatewayNamePrefix is the shared prefix every gateway this suite creates
// carries (e2e-gw-, e2e-sbx-, e2e-del-, e2e-padmin-del-, e2e-padmin-create-).
// The startup sweep matches on it to reap gateways leaked by a prior run.
const e2eGatewayNamePrefix = "e2e-"

// staleGatewaySweepAge is the minimum age a gateway must reach before the startup
// sweep is allowed to delete it. The longest suite run is TestMatrix at
// -timeout 45m, so a concurrently running promotion-gate suite can legitimately
// hold gateways up to roughly that age; the one-hour threshold keeps those fresh
// gateways safe while still reaping anything an abnormally terminated prior run
// leaked.
const staleGatewaySweepAge = time.Hour

// shouldSweepGateway reports whether a gateway is a stale e2e orphan the startup
// sweep may delete: its name carries the e2e- prefix and it was created longer
// ago than minAge. A gateway whose CreatedAt is nil (age unknown) is treated as
// NOT stale, so an unexpectedly shaped list response never causes indiscriminate
// deletion.
func shouldSweepGateway(name string, createdAt *time.Time, now time.Time, minAge time.Duration) bool {
	if !strings.HasPrefix(name, e2eGatewayNamePrefix) {
		return false
	}
	if createdAt == nil {
		return false
	}
	return now.Sub(*createdAt) >= minAge
}

// sweepStaleGateways is the self-healing startup safety net for abnormally
// terminated prior runs. A normal pass or fail deletes every tracked gateway in
// TearDownSuite, but a hard kill (a `go test -timeout` panic, a pod
// activeDeadlineSeconds SIGKILL, or Ctrl-C) never runs the deferred teardown and
// leaks the run's in-flight gateways; over many promotion-gate runs these
// accumulate. This lists every gateway and deletes any e2e- gateway older than
// staleGatewaySweepAge. The age guard keeps a concurrently running suite's fresh
// gateways safe (see staleGatewaySweepAge). It is best-effort and non-fatal: all
// errors are logged and never fail the suite, and the whole sweep is skipped when
// E2E_SKIP_CLEANUP=1 so a diagnostic run keeps prior gateways for inspection.
func (s *E2ESuite) sweepStaleGateways(ctx context.Context) {
	if os.Getenv("E2E_SKIP_CLEANUP") == "1" {
		s.Logf("startup sweep: skipped (E2E_SKIP_CLEANUP=1)")
		return
	}
	if s.admin == nil {
		return
	}
	list, err := s.admin.Gateways().List(ctx, &sdktypes.ListOptions{Size: 1000})
	if err != nil {
		s.Logf("startup sweep: list gateways failed (continuing): %v", err)
		return
	}
	now := time.Now()
	var swept, failed int
	for _, gw := range list.Items {
		if !shouldSweepGateway(gw.Name, gw.CreatedAt, now, staleGatewaySweepAge) {
			continue
		}
		created := gw.CreatedAt.Format(time.RFC3339)
		if err := s.admin.Gateways().Delete(ctx, gw.ID); err != nil {
			var apiErr *sdktypes.APIError
			if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
				continue // already gone; nothing to reap
			}
			failed++
			s.Logf("startup sweep: delete stale gateway %s (%s, created %s) failed (continuing): %v", gw.Name, gw.ID, created, err)
			continue
		}
		swept++
		s.Logf("startup sweep: deleted stale gateway %s (%s, created %s)", gw.Name, gw.ID, created)
	}
	if swept > 0 || failed > 0 {
		s.Logf("startup sweep: deleted %d stale e2e gateway(s), %d deletion(s) failed", swept, failed)
	}
}
