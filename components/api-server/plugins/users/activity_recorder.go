package users

import (
	"context"
	"sync"
	"time"

	"github.com/golang/glog"
)

type UserActivityRecorder interface {
	RecordDailyActivity(ctx context.Context, userID string, at time.Time)
}

type userActivityRecorder struct {
	dao UserActivityDao

	dayMu sync.Mutex
	day   time.Time
	users map[string]struct{}
}

var _ UserActivityRecorder = (*userActivityRecorder)(nil)

func NewUserActivityRecorder(dao UserActivityDao) UserActivityRecorder {
	return &userActivityRecorder{
		dao:   dao,
		users: make(map[string]struct{}),
	}
}

func (r *userActivityRecorder) RecordDailyActivity(ctx context.Context, userID string, at time.Time) {
	if userID == "" {
		return
	}

	activityDay := utcCalendarDate(at)

	r.dayMu.Lock()
	if !activityDay.Equal(r.day) {
		r.day = activityDay
		r.users = make(map[string]struct{})
	}
	if _, seen := r.users[userID]; seen {
		r.dayMu.Unlock()
		return
	}
	r.users[userID] = struct{}{}
	r.dayMu.Unlock()

	if err := r.dao.UpsertDailyActivity(ctx, userID, at); err != nil {
		r.dayMu.Lock()
		if activityDay.Equal(r.day) {
			delete(r.users, userID)
		}
		r.dayMu.Unlock()
		glog.Warningf("user daily activity recording failed for %q: %v", userID, err)
	}
}
