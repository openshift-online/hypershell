package users

import (
	"context"
	"time"

	"github.com/golang/glog"
)

type UserActivityRecorder interface {
	RecordDailyActivity(ctx context.Context, userID string, at time.Time)
}

type userActivityRecorder struct {
	dao UserActivityDao
}

var _ UserActivityRecorder = (*userActivityRecorder)(nil)

func NewUserActivityRecorder(dao UserActivityDao) UserActivityRecorder {
	return &userActivityRecorder{dao: dao}
}

func (r *userActivityRecorder) RecordDailyActivity(ctx context.Context, userID string, at time.Time) {
	if userID == "" {
		return
	}
	if err := r.dao.UpsertDailyActivity(ctx, userID, at); err != nil {
		glog.Warningf("user daily activity recording failed for %q: %v", userID, err)
	}
}
