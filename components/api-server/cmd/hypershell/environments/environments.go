package environments

import (
	"path/filepath"
	"runtime"

	localapi "github.com/openshift-online/hypershell/components/api-server/pkg/api"
	trexapi "github.com/openshift-online/rh-trex-ai/pkg/api"
	pkgenv "github.com/openshift-online/rh-trex-ai/pkg/environments"
	"github.com/openshift-online/rh-trex-ai/pkg/trex"
)

func init() {
	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "../../..")

	trex.Init(trex.Config{
		ServiceName:    "hypershell",
		BasePath:       "/api/hypershell/v1",
		ErrorHref:      "/api/hypershell/v1/errors/",
		MetadataID:     "hypershell",
		ProjectRootDir: projectRoot,
	})

	// The build stamps Version and BuildTime in this module's pkg/api (see the
	// Makefile and Dockerfile ldflags). The framework's metadata handler
	// serves GET /api/hypershell from its own pkg/api variables, so propagate
	// the stamped values to keep the deployed build identifiable.
	trexapi.Version = localapi.Version
	trexapi.BuildTime = localapi.BuildTime

	env := pkgenv.NewEnvironment(nil)
	env.SetEnvironmentImpls(EnvironmentImpls(env))
}

const DevelopmentOidcEnv = "development_oidc"

func EnvironmentImpls(env *pkgenv.Env) map[string]pkgenv.EnvironmentImpl {
	return map[string]pkgenv.EnvironmentImpl{
		pkgenv.DevelopmentEnv:        &DevEnvImpl{Env: env},
		DevelopmentOidcEnv:           &DevOidcEnvImpl{Env: env},
		pkgenv.UnitTestingEnv:        &UnitTestingEnvImpl{Env: env},
		pkgenv.IntegrationTestingEnv: &IntegrationTestingEnvImpl{Env: env},
		pkgenv.ProductionEnv:         &ProductionEnvImpl{Env: env},
	}
}

func GetEnvironmentStrFromEnv() string {
	return pkgenv.GetEnvironmentStrFromEnv()
}

func Environment() *Env {
	return pkgenv.Environment()
}
