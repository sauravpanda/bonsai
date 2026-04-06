package cmd

import (
	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags "-X github.com/sauravpanda/bonsai/cmd.Version=vX.Y.Z".
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:   "bonsai",
	Short: "Git worktree manager",
	Long: `Bonsai helps you manage git worktrees.

As AI-assisted workflows accumulate worktrees, bonsai gives you
audit, clean, and finalize them with ease.`,
	SilenceUsage: true, // don't print usage block on runtime errors
}

func init() {
	// Version is injected by ldflags at build time; apply it here so the
	// assignment happens after ldflags have been processed.
	rootCmd.Version = Version
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
