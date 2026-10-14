package users

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

type testUserDao struct {
	registeredTotal   int64
	createdLast7Days  int64
	createdLast30Days int64
	registeredErr     error
	created7Err       error
	created30Err      error
	gatewayUsage      []UserGatewayUsage
	gatewayUsageErr   error
}

func (d *testUserDao) Get(context.Context, string) (*User, error)            { return nil, nil }
func (d *testUserDao) GetByUsername(context.Context, string) (*User, error)  { return nil, nil }
func (d *testUserDao) Create(context.Context, *User) (*User, error)          { return nil, nil }
func (d *testUserDao) Replace(context.Context, *User) (*User, error)         { return nil, nil }
func (d *testUserDao) Delete(context.Context, string) error                  { return nil }
func (d *testUserDao) Upsert(context.Context, *User) (*User, error)          { return nil, nil }
func (d *testUserDao) FindByIDs(context.Context, []string) (UserList, error) { return nil, nil }
func (d *testUserDao) All(context.Context) (UserList, error)                 { return nil, nil }
func (d *testUserDao) GatewayUsageByUser(context.Context) ([]UserGatewayUsage, error) {
	return d.gatewayUsage, d.gatewayUsageErr
}
func (d *testUserDao) CountRegistered(context.Context) (int64, error) {
	return d.registeredTotal, d.registeredErr
}
func (d *testUserDao) CountCreatedSince(_ context.Context, since time.Time) (int64, error) {
	cutoff7 := time.Now().UTC().Add(-createdLookback7Days)
	if since.After(cutoff7.Add(-time.Minute)) {
		return d.createdLast7Days, d.created7Err
	}
	return d.createdLast30Days, d.created30Err
}

type testActivityDao struct {
	dailyCounts             map[string]int64
	distinctUsersLast7Days  int64
	distinctUsersLast30Days int64
	err                     error
	distinctCountErr        error
}

func (d *testActivityDao) UpsertDailyActivity(context.Context, string, time.Time) error { return nil }
func (d *testActivityDao) DailyUniqueLoginCounts(context.Context, time.Time, time.Time) (map[string]int64, error) {
	if d.err != nil {
		return nil, d.err
	}
	return d.dailyCounts, nil
}
func (d *testActivityDao) CountDistinctUsersWithActivity(_ context.Context, startDate time.Time, endDate time.Time) (int64, error) {
	if d.distinctCountErr != nil {
		return 0, d.distinctCountErr
	}
	today := utcDayStart(time.Now().UTC())
	lookback7DaysStart := today.AddDate(0, 0, -6)
	if !startDate.Before(lookback7DaysStart) {
		return d.distinctUsersLast7Days, nil
	}
	return d.distinctUsersLast30Days, nil
}
func (d *testActivityDao) PruneBefore(context.Context, time.Time) error { return nil }

func TestRegisteredUsersCollectorActivityFailureDoesNotInvalidateRegistrationSeries(t *testing.T) {
	collector := newRegisteredUsersCollector(
		&testUserDao{
			registeredTotal:   42,
			createdLast7Days:  3,
			createdLast30Days: 9,
		},
		&testActivityDao{err: errors.New("daily activity query failed")},
	)

	values, invalid, err := collectRegisteredUserMetrics(collector)
	if err != nil {
		t.Fatal(err)
	}
	if values["hypershell_users_registered_total"] != 42 {
		t.Fatalf("registered_total = %v, want 42", values["hypershell_users_registered_total"])
	}
	if values["hypershell_users_created_last_7_days_total"] != 3 {
		t.Fatalf("created_last_7_days_total = %v, want 3", values["hypershell_users_created_last_7_days_total"])
	}
	if values["hypershell_users_created_last_30_days_total"] != 9 {
		t.Fatalf("created_last_30_days_total = %v, want 9", values["hypershell_users_created_last_30_days_total"])
	}
	if len(invalid) != 3 {
		t.Fatalf("expected 3 invalid login metrics, got %d", len(invalid))
	}
}

func TestUserGatewayUsageCollectorEmitsPerUserPerClusterGaugePairs(t *testing.T) {
	collector := newUserGatewayUsageCollector(&testUserDao{
		gatewayUsage: []UserGatewayUsage{
			{Username: "alice", Cluster: "useast-1", Gateways: 5, Sandboxes: 40},
			{Username: "alice", Cluster: "euwest-1", Gateways: 2, Sandboxes: 24},
			{Username: "bob", Cluster: "useast-1", Gateways: 1, Sandboxes: 0},
			{Username: "carol", Cluster: "", Gateways: 0, Sandboxes: 0},
		},
	})

	samples, err := collectUserUsageMetrics(collector)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][2]float64{
		"alice|useast-1": {5, 40},
		"alice|euwest-1": {2, 24},
		"bob|useast-1":   {1, 0},
		"carol|":         {0, 0},
	}
	if len(samples) != len(want) {
		t.Fatalf("got %d series, want %d", len(samples), len(want))
	}
	for key, expected := range want {
		got, ok := samples[key]
		if !ok {
			t.Fatalf("series %q missing from collected samples", key)
		}
		if got != expected {
			t.Errorf("series %q = {gateways %v, sandboxes %v}, want {gateways %v, sandboxes %v}",
				key, got[0], got[1], expected[0], expected[1])
		}
	}
}

func TestUserGatewayUsageCollectorDaoErrorSurfacesAsInvalidMetric(t *testing.T) {
	collector := newUserGatewayUsageCollector(&testUserDao{
		gatewayUsageErr: errors.New("usage query failed"),
	})

	ch := make(chan prometheus.Metric, 8)
	collector.Collect(ch)
	close(ch)

	invalid := 0
	for metric := range ch {
		dtoMetric := &dto.Metric{}
		if err := metric.Write(dtoMetric); err != nil {
			invalid++
			continue
		}
		if dtoMetric.GetGauge() == nil {
			invalid++
		}
	}
	if invalid != 2 {
		t.Fatalf("expected 2 invalid metrics (one per desc), got %d", invalid)
	}
}

// collectUserUsageMetrics drains the usage collector into a "username|cluster"
// keyed map of {gateways, sandboxes} pairs, failing on any invalid sample.
func collectUserUsageMetrics(collector *userGatewayUsageCollector) (map[string][2]float64, error) {
	ch := make(chan prometheus.Metric, 64)
	collector.Collect(ch)
	close(ch)

	samples := make(map[string][2]float64)
	for metric := range ch {
		dtoMetric := &dto.Metric{}
		if err := metric.Write(dtoMetric); err != nil {
			return nil, err
		}
		if dtoMetric.GetGauge() == nil {
			return nil, errors.New("usage metric is not a gauge")
		}
		username := ""
		cluster := ""
		for _, label := range dtoMetric.GetLabel() {
			switch label.GetName() {
			case "username":
				username = label.GetValue()
			case "cluster":
				cluster = label.GetValue()
			}
		}
		if username == "" {
			return nil, errors.New("usage metric missing username label")
		}
		name := metricFQName(metric.Desc().String())
		// Map values are not addressable; read-modify-write the pair.
		pair := samples[username+"|"+cluster]
		if name == "hypershell_users_gateways_total" {
			pair[0] = dtoMetric.GetGauge().GetValue()
		} else if name == "hypershell_users_sandboxes_total" {
			pair[1] = dtoMetric.GetGauge().GetValue()
		} else {
			return nil, errors.New("unexpected usage metric name: " + name)
		}
		samples[username+"|"+cluster] = pair
	}
	return samples, nil
}

func collectRegisteredUserMetrics(collector prometheus.Collector) (map[string]float64, []string, error) {
	ch := make(chan prometheus.Metric, 64)
	collector.Collect(ch)
	close(ch)

	values := make(map[string]float64)
	var invalid []string
	for metric := range ch {
		dtoMetric := &dto.Metric{}
		if err := metric.Write(dtoMetric); err != nil {
			invalid = append(invalid, metric.Desc().String())
			continue
		}
		if dtoMetric.GetGauge() == nil {
			invalid = append(invalid, metric.Desc().String())
			continue
		}
		name := metricFQName(metric.Desc().String())
		if name == "" {
			return nil, nil, errors.New("metric missing fqName in desc")
		}
		values[name] = dtoMetric.GetGauge().GetValue()
	}
	return values, invalid, nil
}

func metricFQName(desc string) string {
	const prefix = `fqName: "`
	start := strings.Index(desc, prefix)
	if start < 0 {
		return ""
	}
	start += len(prefix)
	end := strings.Index(desc[start:], `"`)
	if end < 0 {
		return ""
	}
	return desc[start : start+end]
}
