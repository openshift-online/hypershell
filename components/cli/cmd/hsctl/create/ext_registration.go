package create

import (
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/create/agentRuntime"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/create/inferenceRoute"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/create/providerBinding"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/create/providerSpec"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/create/sandboxTemplate"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/create/secretSource"

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
