package users

import (
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/pkg/api/presenters"
)

func PresentUser(user *User) openapi.User {
	reference := presenters.PresentReference(user.ID, user)
	return openapi.User{
		Id:        reference.Id,
		Kind:      reference.Kind,
		Href:      reference.Href,
		CreatedAt: openapi.PtrTime(user.CreatedAt),
		UpdatedAt: openapi.PtrTime(user.UpdatedAt),
		Username:  user.Username,
		Email:     user.Email,
		Name:      user.Name,
	}
}

func PresentActivityStats(stats *ActivityStats) openapi.UserActivityStats {
	registrationDaily := make([]openapi.UserDailyCount, 0, len(stats.RegistrationDaily))
	for _, point := range stats.RegistrationDaily {
		registrationDaily = append(registrationDaily, openapi.UserDailyCount{
			Date:  point.Date,
			Count: point.Count,
		})
	}

	activeDaily := make([]openapi.UserDailyCount, 0, len(stats.ActiveDaily))
	for _, point := range stats.ActiveDaily {
		activeDaily = append(activeDaily, openapi.UserDailyCount{
			Date:  point.Date,
			Count: point.Count,
		})
	}

	return openapi.UserActivityStats{
		TotalRegistered:      stats.TotalRegistered,
		RegisteredLast7Days:  stats.RegisteredLast7Days,
		RegisteredLast30Days: stats.RegisteredLast30Days,
		ActiveLast7Days:      stats.ActiveLast7Days,
		ActiveLast30Days:     stats.ActiveLast30Days,
		RegistrationDaily:    registrationDaily,
		ActiveDaily:          activeDaily,
	}
}
