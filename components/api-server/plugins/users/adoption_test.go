package users_test

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/openshift-online/hypershell/components/api-server/plugins/users"
)

func TestBuildUserAdoptionSnapshotRollups(t *testing.T) {
	RegisterTestingT(t)

	evaluationTime := time.Date(2026, 9, 11, 15, 0, 0, 0, time.UTC)
	dailyCounts := map[string]int64{
		"2026-09-10": 1,
		"2026-09-11": 1,
	}

	snapshot := users.BuildUserAdoptionSnapshotForTest(
		10,
		2,
		5,
		dailyCounts,
		evaluationTime,
	)

	Expect(snapshot.UniqueLoginsLast7Days).To(Equal(int64(2)))
	Expect(snapshot.UniqueLoginsLast30Days).To(Equal(int64(2)))
	Expect(snapshot.DailyUniqueLogins[len(snapshot.DailyUniqueLogins)-2].Count).To(Equal(int64(1)))
	Expect(snapshot.DailyUniqueLogins[len(snapshot.DailyUniqueLogins)-1].Count).To(Equal(int64(1)))
}
