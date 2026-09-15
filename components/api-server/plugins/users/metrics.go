package users

import (
	"context"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricsNamespace = "hypershell"
	metricsSubsystem = "users"
)

var userMetricsOnce sync.Once

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
				"Sum of daily unique logins over the last 7 UTC calendar days inclusive of today.",
				nil,
				nil,
			),
			uniqueLoginsLast30Days: prometheus.NewDesc(
				prometheus.BuildFQName(metricsNamespace, metricsSubsystem, "unique_logins_last_30_days_total"),
				"Sum of daily unique logins over the last 30 UTC calendar days inclusive of today.",
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
	} else {
		ch <- prometheus.MustNewConstMetric(
			c.descs.registeredTotal,
			prometheus.GaugeValue,
			float64(totalRegistered),
		)
	}

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
	dailyCounts, err := c.activityDao.DailyUniqueLoginCounts(ctx, startDate, today)
	if err != nil {
		ch <- prometheus.NewInvalidMetric(c.descs.uniqueLoginsDaily, err)
		ch <- prometheus.NewInvalidMetric(c.descs.uniqueLoginsLast7Days, err)
		ch <- prometheus.NewInvalidMetric(c.descs.uniqueLoginsLast30Days, err)
		return
	}

	if err := c.activityDao.PruneBefore(ctx, startDate); err != nil {
		glog.Warningf("user daily activity pruning failed: %v", err)
	}

	loginSnapshot := buildUserAdoptionSnapshot(0, 0, 0, dailyCounts, evaluationTime)
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
