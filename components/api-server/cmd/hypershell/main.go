package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	localapi "github.com/openshift-online/hypershell/components/api-server/pkg/api"
	"github.com/openshift-online/hypershell/components/api-server/pkg/logging"
	pkgcmd "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/cmd"
	pkgenv "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"

	_ "github.com/openshift-online/hypershell/components/api-server/cmd/hypershell/environments"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/agentRuntimes"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/agentWorkspaces"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/gatewayAccess"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/gateways"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/inferenceRoutes"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/managedClusters"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/otel"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/providerBindings"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/providerSpecs"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/rbac"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/repositories"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/roles"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/sandboxTemplates"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/secretSources"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/serviceAccounts"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/users"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/workspaceMemberships"
	_ "github.com/openshift-online/rh-trex-ai/components/api-server/plugins/events"
	_ "github.com/openshift-online/rh-trex-ai/components/api-server/plugins/generic"
)

// Release-flow probe (2026-10-02, inert): a comment-only touch to mint a new
// release bundle and verify the end-to-end path -- Konflux build -> Snapshot ->
// Release -> Renovate bump in hypershell-gitops -> gated promotion through the
// hyp0 -> hyp1 -> hyp2 DAG. No runtime behavior changes.

func main() {
	rootCmd := pkgcmd.NewRootCommand("hypershell", "My service built with TRex library")
	serveCmd := pkgcmd.NewServeCommand(localapi.GetOpenAPISpec)
	// Install the HyperShell logger once flags are parsed and before the
	// framework initializes the environment, so the framework keeps it. It
	// adds the OTel log sink and strips HTTP headers and bodies (pkg/logging).
	// migrate keeps the framework's own stderr logger.
	serveCmd.PreRunE = func(*cobra.Command, []string) error {
		if err := logging.Install(pkgenv.Environment().Config.Logging); err != nil {
			return fmt.Errorf("configure logging: %w", err)
		}
		return registerAdvisoryLockMetrics()
	}
	rootCmd.AddCommand(
		pkgcmd.NewMigrateCommand("hypershell"),
		serveCmd,
	)

	if err := rootCmd.Execute(); err != nil {
		slog.Error("Command failed", "error", err)
		os.Exit(1)
	}
}
