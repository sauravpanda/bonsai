package cmd

import (
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// Version is set at build time via -ldflags "-X github.com/sauravpanda/bonsai/cmd.Version=vX.Y.Z".
var Version = "dev"

var noColor bool

var rootCmd = &cobra.Command{
	Use:   "bonsai",
	Short: "Git worktree manager",
	Long: `Bonsai helps you manage git worktrees.

As AI-assisted workflows accumulate worktrees, bonsai gives you
audit, clean, and finalize them with ease.`,
	SilenceUsage: true, // don't print usage block on runtime errors
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		configureColor(noColor, os.Getenv("NO_COLOR"), term.IsTerminal(int(os.Stdout.Fd())))
	},
}

func init() {
	// Version is injected by ldflags at build time; apply it here so the
	// assignment happens after ldflags have been processed.
	rootCmd.Version = Version
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable ANSI color output")
}

func configureColor(flag bool, noColorEnv string, stdoutIsTTY bool) {
	if shouldDisableColor(flag, noColorEnv, stdoutIsTTY) {
		lipgloss.SetColorProfile(termenv.Ascii)
	}
}

func shouldDisableColor(flag bool, noColorEnv string, stdoutIsTTY bool) bool {
	return flag || noColorEnv != "" || !stdoutIsTTY
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
