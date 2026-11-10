package defaultWorkspaceBootstrap

import (
	"log/slog"
	"sync"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/registry"
)

type ServiceLocator func() Service

func NewServiceLocator(env *environments.Env) ServiceLocator {
	var once sync.Once
	var service Service
	return func() Service {
		once.Do(func() {
			provisioner, err := newControlPlaneProvisionerFromEnvironment()
			if err != nil {
				slog.Warn("Default-workspace bootstrap provisioner is not configured", "error", err)
			}
			service = NewService(NewDao(&env.Database.SessionFactory), provisioner)
		})
		return service
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
