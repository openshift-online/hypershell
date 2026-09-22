package delete

import "github.com/openshift-online/hypershell/components/cli/cmd/hsctl/delete/serviceAccount"

func init() {
	Cmd.AddCommand(serviceAccount.Cmd)
}
