package users

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type countingActivityDao struct {
	mu          sync.Mutex
	upsertCalls int
	err         error
}

func (d *countingActivityDao) UpsertDailyActivity(context.Context, string, time.Time) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.upsertCalls++
	return d.err
}

func (d *countingActivityDao) DailyUniqueLoginCounts(context.Context, time.Time, time.Time) (map[string]int64, error) {
	return nil, nil
}

func (d *countingActivityDao) PruneBefore(context.Context, time.Time) error {
	return nil
}

func (d *countingActivityDao) callCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.upsertCalls
}

func TestUserActivityRecorderSkipsDuplicateSameDayWrites(t *testing.T) {
	dao := &countingActivityDao{}
	recorder := NewUserActivityRecorder(dao)
	ctx := context.Background()
	activityDate := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)

	recorder.RecordDailyActivity(ctx, "user-1", activityDate)
	recorder.RecordDailyActivity(ctx, "user-1", activityDate.Add(2*time.Hour))
	recorder.RecordDailyActivity(ctx, "user-1", activityDate.Add(4*time.Hour))

	if got := dao.callCount(); got != 1 {
		t.Fatalf("expected 1 upsert, got %d", got)
	}
}

func TestUserActivityRecorderWritesOncePerUserPerDay(t *testing.T) {
	dao := &countingActivityDao{}
	recorder := NewUserActivityRecorder(dao)
	ctx := context.Background()
	activityDate := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)

	recorder.RecordDailyActivity(ctx, "user-1", activityDate)
	recorder.RecordDailyActivity(ctx, "user-2", activityDate)

	if got := dao.callCount(); got != 2 {
		t.Fatalf("expected 2 upserts, got %d", got)
	}
}

func TestUserActivityRecorderWritesAgainOnNewUTCDay(t *testing.T) {
	dao := &countingActivityDao{}
	recorder := NewUserActivityRecorder(dao)
	ctx := context.Background()

	recorder.RecordDailyActivity(ctx, "user-1", time.Date(2026, 9, 11, 23, 0, 0, 0, time.UTC))
	recorder.RecordDailyActivity(ctx, "user-1", time.Date(2026, 9, 12, 1, 0, 0, 0, time.UTC))

	if got := dao.callCount(); got != 2 {
		t.Fatalf("expected 2 upserts across UTC days, got %d", got)
	}
}

func TestUserActivityRecorderRetriesAfterPersistenceFailure(t *testing.T) {
	dao := &countingActivityDao{err: errors.New("db unavailable")}
	recorder := NewUserActivityRecorder(dao)
	ctx := context.Background()
	activityDate := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)

	recorder.RecordDailyActivity(ctx, "user-1", activityDate)

	dao.err = nil
	recorder.RecordDailyActivity(ctx, "user-1", activityDate)

	if got := dao.callCount(); got != 2 {
		t.Fatalf("expected retry after failure, got %d upserts", got)
	}
}
