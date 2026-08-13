package users

import (
	"github.com/openshift-online/rh-trex-ai/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
	"github.com/openshift-online/rh-trex-ai/pkg/environments"
	"github.com/openshift-online/rh-trex-ai/pkg/registry"
)

type ServiceLocator func() UserService

func NewServiceLocator(env *environments.Env) ServiceLocator {
	return func() UserService {
		return NewUserService(
			NewUserDao(&env.Database.SessionFactory),
		)
	}
}

func Service(s *environments.Services) UserService {
	if s == nil {
		return nil
	}
	if obj := s.GetService("Users"); obj != nil {
		locator := obj.(ServiceLocator)
		return locator()
	}
	return nil
}

func init() {
	registry.RegisterService("Users", func(env interface{}) interface{} {
		return NewServiceLocator(env.(*environments.Env))
	})

	presenters.RegisterPath(User{}, "users")
	presenters.RegisterPath(&User{}, "users")
	presenters.RegisterKind(User{}, "User")
	presenters.RegisterKind(&User{}, "User")

	db.RegisterMigration(migration())
}
