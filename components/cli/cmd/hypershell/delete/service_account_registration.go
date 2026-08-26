package delete

import "github.com/openshift-online/hypershell/components/cli/cmd/hypershell/delete/serviceAccount"

func init() {
	Cmd.AddCommand(serviceAccount.Cmd)
}
