package update

import (
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/update/platformGatewayPolicy"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/update/platformPolicySettings"

	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/extgroup"
)

// Extension kinds are hand-written (see package extgroup). Most extension kinds
// have no update command; PlatformGatewayPolicy and the platform policy toggle
// do (CLI-EXT-06).
func init() {
	extgroup.Add(Cmd,
		platformGatewayPolicy.Cmd,
		platformPolicySettings.Cmd,
	)
}
