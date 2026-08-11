package environments

import (
	"path/filepath"
	"runtime"

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
