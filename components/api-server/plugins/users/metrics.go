package users

import (
	"context"
	"log/slog"
	"math"
	"sync"
	"sync/atomic"
	"time"

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
	createdDaily           *prometheus.Desc
	inactiveLast30Days     *prometheus.Desc
	uniqueLoginsDaily      *prometheus.Desc
	uniqueLoginsLast7Days  *prometheus.Desc
	uniqueLoginsLast30Days *prometheus.Desc
	retention7Days         *prometheus.Desc
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
			createdDaily: prometheus.NewDesc(
				prometheus.BuildFQName(metricsNamespace, metricsSubsystem, "created_daily_total"),
				"Registered users created on a UTC calendar day.",
				[]string{"created_date"},
				nil,
			),
			inactiveLast30Days: prometheus.NewDesc(
				prometheus.BuildFQName(metricsNamespace, metricsSubsystem, "inactive_last_30_days_total"),
				"Registered users created more than 30 UTC days ago with no API activity in the trailing 30 UTC days.",
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
			retention7Days: prometheus.NewDesc(
				prometheus.BuildFQName(metricsNamespace, metricsSubsystem, "retention_7_days_percent"),
				"Percent of users active in the preceding 7 UTC days who are also active in the current 7 UTC days.",
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
	ch <- c.descs.createdDaily
	ch <- c.descs.inactiveLast30Days
	ch <- c.descs.uniqueLoginsDaily
	ch <- c.descs.uniqueLoginsLast7Days
	ch <- c.descs.uniqueLoginsLast30Days
	ch <- c.descs.retention7Days
}

func (c *registeredUsersCollector) Collect(ch chan<- prometheus.Metric) {
	ctx := context.Background()
	evaluationTime := time.Now().UTC()
	today := utcDayStart(evaluationTime)

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

	dailyCreatedCounts, err := c.userDao.CountCreatedBetweenByDay(ctx, startDateForDays(30, today), today)
	if err != nil {
		ch <- prometheus.NewInvalidMetric(c.descs.createdDaily, err)
	} else {
		for day := 0; day < activityRetentionDays; day++ {
			date := startDateForDays(30, today).AddDate(0, 0, day).Format("2006-01-02")
			ch <- prometheus.MustNewConstMetric(
				c.descs.createdDaily,
				prometheus.GaugeValue,
				float64(dailyCreatedCounts[date]),
				date,
			)
		}
	}

	inactiveUsers, err := c.userDao.CountInactiveSinceWithoutActivityAfter(
		ctx,
		evaluationTime.Add(-createdLookback30Days),
		startDateForDays(30, today),
	)
	if err != nil {
		ch <- prometheus.NewInvalidMetric(c.descs.inactiveLast30Days, err)
	} else {
		ch <- prometheus.MustNewConstMetric(
			c.descs.inactiveLast30Days,
			prometheus.GaugeValue,
			float64(inactiveUsers),
		)
	}

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

	previousCount, retainedCount, err := c.activityDao.CountRetentionWindows(
		ctx,
		startDateForDays(14, today),
		startDateForDays(7, today),
		today,
	)
	switch {
	case err != nil:
		ch <- prometheus.NewInvalidMetric(c.descs.retention7Days, err)
	case previousCount > 0:
		retentionPercent := float64(retainedCount) / float64(previousCount) * 100
		ch <- prometheus.MustNewConstMetric(
			c.descs.retention7Days,
			prometheus.GaugeValue,
			math.Round(retentionPercent*10)/10,
		)
	}
}

func startDateForDays(days int, today time.Time) time.Time {
	return today.AddDate(0, 0, -(days - 1))
}

func maybePruneActivity(ctx context.Context, activityDao UserActivityDao, cutoff time.Time) {
	today := utcDayStart(time.Now().UTC())
	dayNumber := today.Unix() / (24 * 60 * 60)
	if lastActivityPruneDay.Load() == dayNumber {
		return
	}

	if err := activityDao.PruneBefore(ctx, cutoff); err != nil {
		slog.WarnContext(ctx, "user daily activity prune failed", "error", err)
		return
	}

	lastActivityPruneDay.Store(dayNumber)
}
