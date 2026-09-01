package completion

import (
	"os"

	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish]",
	Short: "Generate shell completion scripts",
	Long: "Generate shell completion scripts for the CLI.\n\n" +
		"Examples:\n" +
		"  hsctl completion bash > /etc/bash_completion.d/hsctl\n" +
		"  hsctl completion zsh > \"${fpath[1]}/_hsctl\"\n" +
		"  hsctl completion fish > ~/.config/fish/completions/hsctl.fish",
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"bash", "zsh", "fish"},
	RunE:      run,
}

func run(cmd *cobra.Command, argv []string) error {
	switch argv[0] {
	case "bash":
		return cmd.Root().GenBashCompletion(os.Stdout)
	case "zsh":
		return cmd.Root().GenZshCompletion(os.Stdout)
	case "fish":
		return cmd.Root().GenFishCompletion(os.Stdout, true)
	}
	return nil
}
