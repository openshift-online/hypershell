package test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/golang/glog"
	"github.com/spf13/pflag"

	"github.com/openshift-online/hypershell/components/api-server/cmd/hypershell/environments"
	localapi "github.com/openshift-online/hypershell/components/api-server/pkg/api"
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	pkgserver "github.com/openshift-online/rh-trex-ai/pkg/server"
	"github.com/openshift-online/rh-trex-ai/pkg/testutil"
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
			glog.Fatalf("Unable to add environment flags: %s", err.Error())
		}
		if logLevel := os.Getenv("LOGLEVEL"); logLevel != "" {
			glog.Infof("Using custom loglevel: %s", logLevel)
			if err := pflag.CommandLine.Set("v", logLevel); err != nil {
				glog.Fatalf("Unable to set custom loglevel: %s", err)
			}
		}
		pflag.Parse()

		err = env.Initialize()
		if err != nil {
			glog.Fatalf("Unable to initialize testing environment: %s", err.Error())
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
		glog.Fatalf("Unable to load OpenAPI spec: %s", err)
	}
	helper.APIServer = pkgserver.NewDefaultAPIServer(environments.Environment(), specData)
	listener, err := helper.APIServer.Listen()
	if err != nil {
		glog.Fatalf("Unable to start Test API server: %s", err)
	}
	go func() {
		glog.V(10).Info("Test API server started")
		helper.APIServer.Serve(listener)
		glog.V(10).Info("Test API server stopped")
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
		glog.Fatalf("Unable to start Test gRPC server: %s", err)
	}
	go func() {
		glog.V(10).Info("Test gRPC server started")
		helper.GRPCServer.Serve(listener)
		glog.V(10).Info("Test gRPC server stopped")
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
		glog.V(10).Info("Test Metrics server started")
		helper.MetricsServer.Start()
		glog.V(10).Info("Test Metrics server stopped")
	}()
}

func (helper *Helper) stopMetricsServer() {
	if err := helper.MetricsServer.Stop(); err != nil {
		glog.Fatalf("Unable to stop metrics server: %s", err.Error())
	}
}

func (helper *Helper) startHealthCheckServer() {
	helper.HealthCheckServer = pkgserver.NewDefaultHealthCheckServer(environments.Environment())
	go func() {
		glog.V(10).Info("Test health check server started")
		helper.HealthCheckServer.Start()
		glog.V(10).Info("Test health check server stopped")
	}()
}

func (helper *Helper) RestartServer() {
	if err := helper.stopAPIServer(); err != nil {
		helper.T.Fatalf("Unable to restart API server: %s", err)
	}
	helper.startAPIServer()
	glog.V(10).Info("Test API server restarted")
}

func (helper *Helper) RestartMetricsServer() {
	helper.stopMetricsServer()
	helper.startMetricsServer()
	glog.V(10).Info("Test metrics server restarted")
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
