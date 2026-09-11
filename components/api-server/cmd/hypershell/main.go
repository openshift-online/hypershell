package main

import (
	"flag"
	"fmt"
	"strconv"

	"github.com/golang/glog"
	"github.com/spf13/cobra"

	localapi "github.com/openshift-online/hypershell/components/api-server/pkg/api"
	pkgcmd "github.com/openshift-online/rh-trex-ai/pkg/cmd"

	_ "github.com/openshift-online/hypershell/components/api-server/cmd/hypershell/environments"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/gatewayNetworks"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/gatewayReleases"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/gateways"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/managedClusters"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/managedDatabases"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/otel"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/rbac"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/roles"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/serviceAccounts"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/users"
	_ "github.com/openshift-online/rh-trex-ai/plugins/events"
	_ "github.com/openshift-online/rh-trex-ai/plugins/generic"
)

// rh-trex-ai includes HTTP request headers and request and response bodies at
// verbosity 10. HyperShell has endpoints that return one-time credentials, so
// the API must never enable that logging mode. Keep metadata-only HTTP request
// logging available at lower verbosity levels.
const maximumSafeLogVerbosity = 9

func main() {
	rootCmd := pkgcmd.NewRootCommand("hypershell", "My service built with TRex library")
	rootCmd.PersistentPreRunE = func(*cobra.Command, []string) error {
		return enforceSafeLogVerbosity()
	}
	rootCmd.AddCommand(
		pkgcmd.NewMigrateCommand("hypershell"),
		pkgcmd.NewServeCommand(localapi.GetOpenAPISpec),
	)

	if err := rootCmd.Execute(); err != nil {
		glog.Fatalf("error running command: %v", err)
	}
}

func enforceSafeLogVerbosity() error {
	verbosity := flag.Lookup("v")
	if verbosity == nil {
		return nil
	}
	level, err := strconv.Atoi(verbosity.Value.String())
	if err != nil {
		return fmt.Errorf("parse log verbosity: %w", err)
	}
	if level > maximumSafeLogVerbosity {
		if err := verbosity.Value.Set(strconv.Itoa(maximumSafeLogVerbosity)); err != nil {
			return fmt.Errorf("cap log verbosity: %w", err)
		}
		glog.Warningf("Log verbosity was capped at %d to prevent HTTP header and body logging", maximumSafeLogVerbosity)
	}

	// A vmodule override can enable verbosity 10 even when the global level is
	// capped. Disable it until the framework supports field-level redaction.
	if vmodule := flag.Lookup("vmodule"); vmodule != nil && vmodule.Value.String() != "" {
		if err := vmodule.Value.Set(""); err != nil {
			return fmt.Errorf("disable per-module log verbosity: %w", err)
		}
		glog.Warning("Per-module log verbosity was disabled to prevent HTTP header and body logging")
	}
	return nil
}
