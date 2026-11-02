package e2e

import (
	"fmt"
	"testing"
	"time"

	sdktypes "github.com/openshift-online/hypershell/components/sdk-go/types"
)

func TestShouldSweepGateway(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	ptr := func(d time.Duration) *time.Time {
		at := now.Add(d)
		return &at
	}

	tests := []struct {
		name      string
		gwName    string
		createdAt *time.Time
		want      bool
	}{
		{
			name:      "stale e2e gateway is swept",
			gwName:    "e2e-gw-12345",
			createdAt: ptr(-2 * time.Hour),
			want:      true,
		},
		{
			name:      "stale e2e deletion victim is swept",
			gwName:    "e2e-padmin-del-12345",
			createdAt: ptr(-90 * time.Minute),
			want:      true,
		},
		{
			name:      "fresh e2e gateway is spared (concurrent run)",
			gwName:    "e2e-gw-12345",
			createdAt: ptr(-5 * time.Minute),
			want:      false,
		},
		{
			name:      "e2e gateway from a concurrent long run is spared",
			gwName:    "e2e-sbx-99999",
			createdAt: ptr(-40 * time.Minute),
			want:      false,
		},
		{
			name:      "non-e2e gateway is never swept",
			gwName:    "prod-gateway",
			createdAt: ptr(-72 * time.Hour),
			want:      false,
		},
		{
			name:      "e2e gateway with no timestamp is spared",
			gwName:    "e2e-del-12345",
			createdAt: nil,
			want:      false,
		},
		{
			name:      "exactly at the age threshold is swept",
			gwName:    "e2e-gw-1",
			createdAt: ptr(-staleGatewaySweepAge),
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldSweepGateway(tt.gwName, tt.createdAt, now, staleGatewaySweepAge)
			if got != tt.want {
				t.Errorf("shouldSweepGateway(%q, %v) = %v, want %v", tt.gwName, tt.createdAt, got, tt.want)
			}
		})
	}
}

func gw(id, name string, createdAt *time.Time) sdktypes.Gateway {
	return sdktypes.Gateway{
		ObjectReference: sdktypes.ObjectReference{ID: id, CreatedAt: createdAt},
		Name:            name,
	}
}

// TestCollectStaleGatewaysPaginates proves the sweep reaches orphans beyond the
// first page. Gateway lists are newest-first, so the one stale e2e orphan sits on
// the last page behind fresh gateways; a single-page sweep would miss it.
func TestCollectStaleGatewaysPaginates(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) *time.Time {
		v := now.Add(d)
		return &v
	}

	pages := [][]sdktypes.Gateway{
		{
			gw("1", "e2e-gw-fresh", at(-1*time.Minute)),
			gw("2", "prod-a", at(-2*time.Minute)),
		},
		{
			gw("3", "e2e-gw-recent", at(-30*time.Minute)),
		},
		{
			gw("4", "e2e-gw-stale", at(-3*time.Hour)),
			gw("5", "prod-old", at(-4*time.Hour)),
		},
	}
	fetchPage := func(page int) (*sdktypes.GatewayList, error) {
		if page < 1 || page > len(pages) {
			return &sdktypes.GatewayList{}, nil // beyond the last page: empty, stops paging
		}
		return &sdktypes.GatewayList{Items: pages[page-1]}, nil
	}

	stale, err := collectStaleGateways(fetchPage, now, staleGatewaySweepAge)
	if err != nil {
		t.Fatalf("collectStaleGateways: %v", err)
	}
	if len(stale) != 1 || stale[0].id != "4" {
		t.Fatalf("expected only the beyond-first-page stale e2e orphan (id 4), got %+v", stale)
	}
}

// TestCollectStaleGatewaysFetchErrorReturnsPartial confirms a mid-pagination
// listing failure still surfaces the orphans found on earlier pages so the sweep
// can reap them, and reports the error.
func TestCollectStaleGatewaysFetchErrorReturnsPartial(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	staleAt := now.Add(-3 * time.Hour)

	fetchPage := func(page int) (*sdktypes.GatewayList, error) {
		if page == 1 {
			return &sdktypes.GatewayList{Items: []sdktypes.Gateway{gw("1", "e2e-gw-stale", &staleAt)}}, nil
		}
		return nil, fmt.Errorf("boom")
	}

	stale, err := collectStaleGateways(fetchPage, now, staleGatewaySweepAge)
	if err == nil {
		t.Fatal("expected a fetch error from page 2")
	}
	if len(stale) != 1 || stale[0].id != "1" {
		t.Fatalf("expected the page-1 stale orphan preserved, got %+v", stale)
	}
}
