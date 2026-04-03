package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "bonsai",
	Short: "Git worktree manager",
	Long: `Bonsai helps you manage git worktrees.

As AI-assisted workflows accumulate worktrees, bonsai gives you
audit, clean, and finalize them with ease.`,
	SilenceUsage: true, // don't print usage block on runtime errors
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
