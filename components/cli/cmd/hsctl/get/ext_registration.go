package get

import (
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/get/agentRuntime"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/get/inferenceRoute"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/get/providerBinding"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/get/providerSpec"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/get/sandboxTemplate"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/get/secretSource"

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
