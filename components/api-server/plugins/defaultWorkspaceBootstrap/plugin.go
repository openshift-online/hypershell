package defaultWorkspaceBootstrap

import (
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/registry"
)

type ServiceLocator func() Service

func NewServiceLocator(env *environments.Env) ServiceLocator {
	return func() Service {
		provisioner, err := newControlPlaneProvisionerFromEnvironment()
		if err != nil || provisioner == nil {
			return NewService(NewDao(&env.Database.SessionFactory), nil)
		}
		return NewService(NewDao(&env.Database.SessionFactory), provisioner)
	}
}

func ServiceFrom(s *environments.Services) Service {
	if s == nil {
		return nil
	}
	if object := s.GetService("DefaultWorkspaceBootstrap"); object != nil {
		return object.(ServiceLocator)()
	}
	return nil
}

func init() {
	registry.RegisterService("DefaultWorkspaceBootstrap", func(env interface{}) interface{} {
		return NewServiceLocator(env.(*environments.Env))
	})
	db.RegisterMigration(migration())
}
