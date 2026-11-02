package e2e

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	sdktypes "github.com/openshift-online/hypershell/components/sdk-go/types"
)

// sweepPageSize is the gateway list page size the startup sweep requests. Gateway
// lists are sorted newest-first, so stale orphans land on later pages; the sweep
// must page to exhaustion (see collectStaleGateways) regardless of this value.
const sweepPageSize = 100

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

// staleGateway is a single gateway the startup sweep has decided to delete. The
// created string is pre-formatted (from a non-nil CreatedAt, guaranteed by
// shouldSweepGateway) only for logging.
type staleGateway struct {
	id      string
	name    string
	created string
}

// collectStaleGateways pages through fetchPage (pages are 1-based) and returns
// every gateway shouldSweepGateway flags as a stale e2e orphan. It stops at the
// first empty page, matching the generated SDK iterator's termination contract.
// Gateway lists are sorted newest-first, so stale orphans land on later pages:
// paging to exhaustion is required or the oldest orphans (exactly the ones this
// sweep exists to reap) are never seen. A fetch error stops paging and is
// returned alongside the gateways collected from earlier pages, so a
// mid-pagination failure still lets the caller reap what was already found.
func collectStaleGateways(fetchPage func(page int) (*sdktypes.GatewayList, error), now time.Time, minAge time.Duration) ([]staleGateway, error) {
	var stale []staleGateway
	for page := 1; ; page++ {
		list, err := fetchPage(page)
		if err != nil {
			return stale, fmt.Errorf("list gateways page %d: %w", page, err)
		}
		if len(list.Items) == 0 {
			return stale, nil
		}
		for _, gw := range list.Items {
			if !shouldSweepGateway(gw.Name, gw.CreatedAt, now, minAge) {
				continue
			}
			stale = append(stale, staleGateway{id: gw.ID, name: gw.Name, created: gw.CreatedAt.Format(time.RFC3339)})
		}
	}
}

// sweepStaleGateways is the self-healing startup safety net for abnormally
// terminated prior runs. A normal pass or fail deletes every tracked gateway in
// TearDownSuite, but a hard kill (a `go test -timeout` panic, a pod
// activeDeadlineSeconds SIGKILL, or Ctrl-C) never runs the deferred teardown and
// leaks the run's in-flight gateways; over many promotion-gate runs these
// accumulate. This pages through every gateway and deletes any e2e- gateway older
// than staleGatewaySweepAge. The age guard keeps a concurrently running suite's
// fresh gateways safe (see staleGatewaySweepAge). It is best-effort and
// non-fatal: all errors are logged and never fail the suite (a listing error
// still deletes the orphans collected before it), and the whole sweep is skipped
// when E2E_SKIP_CLEANUP=1 so a diagnostic run keeps prior gateways for inspection.
func (s *E2ESuite) sweepStaleGateways(ctx context.Context) {
	if os.Getenv("E2E_SKIP_CLEANUP") == "1" {
		s.Logf("startup sweep: skipped (E2E_SKIP_CLEANUP=1)")
		return
	}
	if s.admin == nil {
		return
	}
	fetchPage := func(page int) (*sdktypes.GatewayList, error) {
		return s.admin.Gateways().List(ctx, &sdktypes.ListOptions{Page: page, Size: sweepPageSize})
	}
	stale, err := collectStaleGateways(fetchPage, time.Now(), staleGatewaySweepAge)
	if err != nil {
		// Paging stopped early; still delete whatever earlier pages yielded.
		s.Logf("startup sweep: listing gateways failed (continuing with %d collected): %v", len(stale), err)
	}
	var swept, failed int
	for _, gw := range stale {
		if delErr := s.admin.Gateways().Delete(ctx, gw.id); delErr != nil {
			var apiErr *sdktypes.APIError
			if errors.As(delErr, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
				continue // already gone; nothing to reap
			}
			failed++
			s.Logf("startup sweep: delete stale gateway %s (%s, created %s) failed (continuing): %v", gw.name, gw.id, gw.created, delErr)
			continue
		}
		swept++
		s.Logf("startup sweep: deleted stale gateway %s (%s, created %s)", gw.name, gw.id, gw.created)
	}
	if swept > 0 || failed > 0 {
		s.Logf("startup sweep: deleted %d stale e2e gateway(s), %d deletion(s) failed", swept, failed)
	}
}
