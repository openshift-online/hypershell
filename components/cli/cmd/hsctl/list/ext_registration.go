package list

import (
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/list/agentRuntimes"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/list/inferenceRoutes"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/list/providerBindings"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/list/providerSpecs"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/list/sandboxTemplates"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/list/secretSources"

	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/extgroup"
)

// Extension kinds are hand-written (see package extgroup).
func init() {
	extgroup.Add(Cmd,
		agentRuntimes.Cmd,
		sandboxTemplates.Cmd,
		providerSpecs.Cmd,
		providerBindings.Cmd,
		inferenceRoutes.Cmd,
		secretSources.Cmd,
	)
}
