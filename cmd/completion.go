package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish]",
	Short: "Generate shell completion script",
	Long: `Generate a shell completion script for bonsai.

Bash:
  source <(bonsai completion bash)

  # To load completions for each session, add to ~/.bashrc:
  bonsai completion bash > ~/.bash_completion.d/bonsai

Zsh:
  # If shell completion is not already enabled, execute once:
  echo "autoload -U compinit; compinit" >> ~/.zshrc

  # Load completions for each session:
  bonsai completion zsh > "${fpath[1]}/_bonsai"

Fish:
  bonsai completion fish | source

  # To load for each session:
  bonsai completion fish > ~/.config/fish/completions/bonsai.fish
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return cmd.Root().GenBashCompletion(os.Stdout)
		case "zsh":
			return cmd.Root().GenZshCompletion(os.Stdout)
		case "fish":
			return cmd.Root().GenFishCompletion(os.Stdout, true)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}
