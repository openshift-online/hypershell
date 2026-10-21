package test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/spf13/pflag"

	"github.com/openshift-online/hypershell/components/api-server/cmd/hypershell/environments"
	localapi "github.com/openshift-online/hypershell/components/api-server/pkg/api"
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	pkgserver "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/server"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/testutil"
)

const (
	apiPort  = ":8777"
	grpcPort = ":9777"
)

var (
	helper *Helper
	once   sync.Once
)

type TimeFunc func() time.Time

type Helper struct {
	testutil.BaseHelper
	APIServer         pkgserver.Server
	GRPCServer        pkgserver.Server
	ControllersServer *pkgserver.ControllersServer
	MetricsServer     pkgserver.Server
	HealthCheckServer pkgserver.Server
	TimeFunc          TimeFunc
	teardowns         []func() error
}

func NewHelper(t *testing.T) *Helper {
	once.Do(func() {
		env := environments.Environment()
		err := env.AddFlags(pflag.CommandLine)
		if err != nil {
			slog.Error("Unable to add environment flags", "error", err)
			os.Exit(1)
		}
		if logLevel := os.Getenv("LOGLEVEL"); logLevel != "" {
			slog.Info("Using custom log level", "log_level", logLevel)
			// A number is the old glog verbosity: it sets the deprecated -v.
			flagName := "log-level"
			if _, err := strconv.Atoi(logLevel); err == nil {
				flagName = "v"
			}
			if err := pflag.CommandLine.Set(flagName, logLevel); err != nil {
				slog.Error("Unable to set log level", "log_level", logLevel, "error", err)
				os.Exit(1)
			}
		}
		pflag.Parse()

		err = env.Initialize()
		if err != nil {
			slog.Error("Unable to initialize testing environment", "error", err)
			os.Exit(1)
		}

		base := testutil.NewBaseHelper(
			environments.Environment().Config,
			environments.Environment().Database.SessionFactory,
		)

		helper = &Helper{
			BaseHelper: *base,
		}

		_, jwkMockTeardown := helper.StartJWKCertServerMock()
		helper.teardowns = []func() error{
			helper.stopControllersServer,
			helper.CleanDB,
			jwkMockTeardown,
			helper.stopGRPCServer,
			helper.stopAPIServer,
			helper.teardownEnv,
		}
		helper.initControllersServer()
		helper.startAPIServer()
		helper.startGRPCServer()
		helper.startMetricsServer()
		helper.startHealthCheckServer()
	})
	helper.T = t
	return helper
}

func (helper *Helper) Env() *environments.Env {
	return environments.Environment()
}

func (helper *Helper) teardownEnv() error {
	helper.Env().Teardown()
	return nil
}

func (helper *Helper) Teardown() {
	for _, f := range helper.teardowns {
		err := f()
		if err != nil {
			helper.T.Errorf("error running teardown func: %s", err)
		}
	}
}

func (helper *Helper) startAPIServer() {
	specData, err := localapi.GetOpenAPISpec()
	if err != nil {
		slog.Error("Unable to load OpenAPI spec", "error", err)
		os.Exit(1)
	}
	helper.APIServer = pkgserver.NewDefaultAPIServer(environments.Environment(), specData)
	listener, err := helper.APIServer.Listen()
	if err != nil {
		slog.Error("Unable to start Test API server", "error", err)
		os.Exit(1)
	}
	go func() {
		slog.Debug("Test API server started")
		helper.APIServer.Serve(listener)
		slog.Debug("Test API server stopped")
	}()
}

func (helper *Helper) stopAPIServer() error {
	if err := helper.APIServer.Stop(); err != nil {
		return fmt.Errorf("unable to stop api server: %s", err.Error())
	}
	return nil
}

func (helper *Helper) startGRPCServer() {
	env := environments.Environment()
	env.Config.GRPC.BindAddress = grpcPort
	helper.GRPCServer = pkgserver.NewDefaultGRPCServer(env)
	listener, err := helper.GRPCServer.Listen()
	if err != nil {
		slog.Error("Unable to start Test gRPC server", "error", err)
		os.Exit(1)
	}
	go func() {
		slog.Debug("Test gRPC server started")
		helper.GRPCServer.Serve(listener)
		slog.Debug("Test gRPC server stopped")
	}()
}

func (helper *Helper) stopGRPCServer() error {
	if helper.GRPCServer != nil {
		if err := helper.GRPCServer.Stop(); err != nil {
			return fmt.Errorf("unable to stop grpc server: %s", err.Error())
		}
	}
	return nil
}

func (helper *Helper) GRPCAddress() string {
	return "localhost" + grpcPort
}

func (helper *Helper) initControllersServer() {
	env := environments.Environment()
	helper.ControllersServer = pkgserver.NewDefaultControllersServer(env)
}

func (helper *Helper) StartControllersServer() {
	go helper.ControllersServer.Start()
}

func (helper *Helper) stopControllersServer() error {
	if helper.ControllersServer != nil {
		helper.ControllersServer.Stop()
	}
	return nil
}

func (helper *Helper) startMetricsServer() {
	helper.MetricsServer = pkgserver.NewDefaultMetricsServer(environments.Environment())
	go func() {
		slog.Debug("Test Metrics server started")
		helper.MetricsServer.Start()
		slog.Debug("Test Metrics server stopped")
	}()
}

func (helper *Helper) stopMetricsServer() {
	if err := helper.MetricsServer.Stop(); err != nil {
		slog.Error("Unable to stop metrics server", "error", err)
		os.Exit(1)
	}
}

func (helper *Helper) startHealthCheckServer() {
	helper.HealthCheckServer = pkgserver.NewDefaultHealthCheckServer(environments.Environment())
	go func() {
		slog.Debug("Test health check server started")
		helper.HealthCheckServer.Start()
		slog.Debug("Test health check server stopped")
	}()
}

func (helper *Helper) RestartServer() {
	if err := helper.stopAPIServer(); err != nil {
		helper.T.Fatalf("Unable to restart API server: %s", err)
	}
	helper.startAPIServer()
	slog.Debug("Test API server restarted")
}

func (helper *Helper) RestartMetricsServer() {
	helper.stopMetricsServer()
	helper.startMetricsServer()
	slog.Debug("Test metrics server restarted")
}

func (helper *Helper) NewApiClient() *openapi.APIClient {
	cfg := openapi.NewConfiguration()
	client := openapi.NewAPIClient(cfg)
	return client
}

func (helper *Helper) NewAuthenticatedContext(account *testutil.TestAccount) context.Context {
	tokenString := helper.CreateJWTString(account)
	return context.WithValue(context.Background(), openapi.ContextAccessToken, tokenString)
}

func (helper *Helper) OpenapiError(err error) openapi.Error {
	generic := err.(openapi.GenericOpenAPIError)
	var exErr openapi.Error
	jsonErr := json.Unmarshal(generic.Body(), &exErr)
	if jsonErr != nil {
		helper.T.Errorf("Unable to convert error response to openapi error: %s", jsonErr)
	}
	return exErr
}
