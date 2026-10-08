package delete

import (
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/delete/agentRuntime"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/delete/inferenceRoute"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/delete/providerBinding"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/delete/providerSpec"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/delete/sandboxTemplate"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/delete/secretSource"

	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/extgroup"
)

// Extension kinds are hand-written (see package extgroup).
func init() {
	extgroup.Add(Cmd,
		agentRuntime.Cmd,
		sandboxTemplate.Cmd,
		providerSpec.Cmd,
		providerBinding.Cmd,
		inferenceRoute.Cmd,
		secretSource.Cmd,
	)
}
