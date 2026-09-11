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
