package users_test

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/openshift-online/hypershell/components/api-server/plugins/users"
	"github.com/openshift-online/hypershell/components/api-server/test"
	"github.com/openshift-online/rh-trex-ai/pkg/environments"
)

func TestUserDailyActivityUpsertIsIdempotent(t *testing.T) {
	h, _ := test.RegisterIntegration(t)

	userService := users.Service(&h.Env().Services)
	Expect(userService).NotTo(BeNil())

	userID, err := userService.UpsertByUsername(context.Background(), "activity-user", nil, nil)
	Expect(err).NotTo(HaveOccurred())

	activityDao := users.NewUserActivityDao(&environments.Environment().Database.SessionFactory)
	activityDate := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)

	Expect(activityDao.UpsertDailyActivity(context.Background(), userID, activityDate)).To(Succeed())
	Expect(activityDao.UpsertDailyActivity(context.Background(), userID, activityDate)).To(Succeed())

	counts, err := activityDao.DailyUniqueLoginCounts(
		context.Background(),
		activityDate,
		activityDate,
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(counts["2026-09-11"]).To(Equal(int64(1)))
}

func TestUserDailyActivityDistinctUsersAcrossDays(t *testing.T) {
	h, _ := test.RegisterIntegration(t)

	userService := users.Service(&h.Env().Services)
	Expect(userService).NotTo(BeNil())

	userID, err := userService.UpsertByUsername(context.Background(), "repeat-activity-user", nil, nil)
	Expect(err).NotTo(HaveOccurred())

	activityDao := users.NewUserActivityDao(&environments.Environment().Database.SessionFactory)
	firstDay := time.Date(2026, 9, 10, 8, 0, 0, 0, time.UTC)
	secondDay := time.Date(2026, 9, 11, 9, 0, 0, 0, time.UTC)
	Expect(activityDao.UpsertDailyActivity(context.Background(), userID, firstDay)).To(Succeed())
	Expect(activityDao.UpsertDailyActivity(context.Background(), userID, secondDay)).To(Succeed())

	distinctCount, err := activityDao.CountDistinctUsersWithActivity(
		context.Background(),
		firstDay,
		secondDay,
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(distinctCount).To(Equal(int64(1)))
}

func TestUserDailyActivityPruneRemovesExpiredRows(t *testing.T) {
	h, _ := test.RegisterIntegration(t)

	userService := users.Service(&h.Env().Services)
	Expect(userService).NotTo(BeNil())

	userID, err := userService.UpsertByUsername(context.Background(), "prune-user", nil, nil)
	Expect(err).NotTo(HaveOccurred())

	activityDao := users.NewUserActivityDao(&environments.Environment().Database.SessionFactory)
	expiredDate := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	Expect(activityDao.UpsertDailyActivity(context.Background(), userID, expiredDate)).To(Succeed())

	cutoff := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	Expect(activityDao.PruneBefore(context.Background(), cutoff)).To(Succeed())

	counts, err := activityDao.DailyUniqueLoginCounts(
		context.Background(),
		expiredDate,
		expiredDate,
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(counts).To(BeEmpty())
}
