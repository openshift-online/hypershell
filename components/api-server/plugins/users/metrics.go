package users

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricsNamespace = "hypershell"
	metricsSubsystem = "users"

	createdLookback7Days  = 7 * 24 * time.Hour
	createdLookback30Days = 30 * 24 * time.Hour
)

var (
	userMetricsOnce      sync.Once
	lastActivityPruneDay atomic.Int64
)

// RegisterUserMetrics registers Prometheus gauges for registered user adoption.
// Safe to call multiple times.
func RegisterUserMetrics(userDao UserDao, activityDao UserActivityDao) {
	userMetricsOnce.Do(func() {
		prometheus.MustRegister(newRegisteredUsersCollector(userDao, activityDao))
	})
}

type registeredUsersCollector struct {
	userDao     UserDao
	activityDao UserActivityDao
	descs       registeredUsersMetricDescs
}

type registeredUsersMetricDescs struct {
	registeredTotal        *prometheus.Desc
	createdLast7Days       *prometheus.Desc
	createdLast30Days      *prometheus.Desc
	uniqueLoginsDaily      *prometheus.Desc
	uniqueLoginsLast7Days  *prometheus.Desc
	uniqueLoginsLast30Days *prometheus.Desc
}

func newRegisteredUsersCollector(userDao UserDao, activityDao UserActivityDao) *registeredUsersCollector {
	return &registeredUsersCollector{
		userDao:     userDao,
		activityDao: activityDao,
		descs: registeredUsersMetricDescs{
			registeredTotal: prometheus.NewDesc(
				prometheus.BuildFQName(metricsNamespace, metricsSubsystem, "registered_total"),
				"Total registered users.",
				nil,
				nil,
			),
			createdLast7Days: prometheus.NewDesc(
				prometheus.BuildFQName(metricsNamespace, metricsSubsystem, "created_last_7_days_total"),
				"Registered users created in the last 7 days.",
				nil,
				nil,
			),
			createdLast30Days: prometheus.NewDesc(
				prometheus.BuildFQName(metricsNamespace, metricsSubsystem, "created_last_30_days_total"),
				"Registered users created in the last 30 days.",
				nil,
				nil,
			),
			uniqueLoginsDaily: prometheus.NewDesc(
				prometheus.BuildFQName(metricsNamespace, metricsSubsystem, "unique_logins_daily_total"),
				"Daily unique registered users with API activity.",
				[]string{"activity_date"},
				nil,
			),
			uniqueLoginsLast7Days: prometheus.NewDesc(
				prometheus.BuildFQName(metricsNamespace, metricsSubsystem, "unique_logins_last_7_days_total"),
				"Distinct registered users with API activity over the last 7 UTC calendar days inclusive of today.",
				nil,
				nil,
			),
			uniqueLoginsLast30Days: prometheus.NewDesc(
				prometheus.BuildFQName(metricsNamespace, metricsSubsystem, "unique_logins_last_30_days_total"),
				"Distinct registered users with API activity over the last 30 UTC calendar days inclusive of today.",
				nil,
				nil,
			),
		},
	}
}

func (c *registeredUsersCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.descs.registeredTotal
	ch <- c.descs.createdLast7Days
	ch <- c.descs.createdLast30Days
	ch <- c.descs.uniqueLoginsDaily
	ch <- c.descs.uniqueLoginsLast7Days
	ch <- c.descs.uniqueLoginsLast30Days
}

func (c *registeredUsersCollector) Collect(ch chan<- prometheus.Metric) {
	ctx := context.Background()
	evaluationTime := time.Now().UTC()

	totalRegistered, err := c.userDao.CountRegistered(ctx)
	if err != nil {
		ch <- prometheus.NewInvalidMetric(c.descs.registeredTotal, err)
		return
	}
	ch <- prometheus.MustNewConstMetric(
		c.descs.registeredTotal,
		prometheus.GaugeValue,
		float64(totalRegistered),
	)

	createdLast7Days, err := c.userDao.CountCreatedSince(ctx, evaluationTime.Add(-createdLookback7Days))
	if err != nil {
		ch <- prometheus.NewInvalidMetric(c.descs.createdLast7Days, err)
	} else {
		ch <- prometheus.MustNewConstMetric(
			c.descs.createdLast7Days,
			prometheus.GaugeValue,
			float64(createdLast7Days),
		)
	}

	createdLast30Days, err := c.userDao.CountCreatedSince(ctx, evaluationTime.Add(-createdLookback30Days))
	if err != nil {
		ch <- prometheus.NewInvalidMetric(c.descs.createdLast30Days, err)
	} else {
		ch <- prometheus.MustNewConstMetric(
			c.descs.createdLast30Days,
			prometheus.GaugeValue,
			float64(createdLast30Days),
		)
	}

	today := utcDayStart(evaluationTime)
	startDate := today.AddDate(0, 0, -(activityRetentionDays - 1))
	lookback7DaysStart := today.AddDate(0, 0, -6)
	lookback30DaysStart := today.AddDate(0, 0, -29)

	dailyCounts, err := c.activityDao.DailyUniqueLoginCounts(ctx, startDate, today)
	if err != nil {
		ch <- prometheus.NewInvalidMetric(c.descs.uniqueLoginsDaily, err)
		ch <- prometheus.NewInvalidMetric(c.descs.uniqueLoginsLast7Days, err)
		ch <- prometheus.NewInvalidMetric(c.descs.uniqueLoginsLast30Days, err)
		return
	}

	uniqueLoginsLast7Days, err := c.activityDao.CountDistinctUsersWithActivity(ctx, lookback7DaysStart, today)
	if err != nil {
		ch <- prometheus.NewInvalidMetric(c.descs.uniqueLoginsDaily, err)
		ch <- prometheus.NewInvalidMetric(c.descs.uniqueLoginsLast7Days, err)
		ch <- prometheus.NewInvalidMetric(c.descs.uniqueLoginsLast30Days, err)
		return
	}

	uniqueLoginsLast30Days, err := c.activityDao.CountDistinctUsersWithActivity(ctx, lookback30DaysStart, today)
	if err != nil {
		ch <- prometheus.NewInvalidMetric(c.descs.uniqueLoginsDaily, err)
		ch <- prometheus.NewInvalidMetric(c.descs.uniqueLoginsLast7Days, err)
		ch <- prometheus.NewInvalidMetric(c.descs.uniqueLoginsLast30Days, err)
		return
	}

	maybePruneActivity(ctx, c.activityDao, startDate)

	loginSnapshot := buildUserAdoptionSnapshot(
		0,
		0,
		0,
		dailyCounts,
		uniqueLoginsLast7Days,
		uniqueLoginsLast30Days,
		evaluationTime,
	)
	for _, day := range loginSnapshot.DailyUniqueLogins {
		ch <- prometheus.MustNewConstMetric(
			c.descs.uniqueLoginsDaily,
			prometheus.GaugeValue,
			float64(day.Count),
			day.Date,
		)
	}
	ch <- prometheus.MustNewConstMetric(
		c.descs.uniqueLoginsLast7Days,
		prometheus.GaugeValue,
		float64(loginSnapshot.UniqueLoginsLast7Days),
	)
	ch <- prometheus.MustNewConstMetric(
		c.descs.uniqueLoginsLast30Days,
		prometheus.GaugeValue,
		float64(loginSnapshot.UniqueLoginsLast30Days),
	)
}

func maybePruneActivity(ctx context.Context, activityDao UserActivityDao, cutoff time.Time) {
	today := utcDayStart(time.Now().UTC())
	dayNumber := today.Unix() / (24 * 60 * 60)
	if lastActivityPruneDay.Load() == dayNumber {
		return
	}

	if err := activityDao.PruneBefore(ctx, cutoff); err != nil {
		glog.Warningf("user daily activity prune failed: %v", err)
		return
	}

	lastActivityPruneDay.Store(dayNumber)
}
