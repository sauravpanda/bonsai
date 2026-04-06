package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/sauravpanda/bonsai/internal/config"
	"github.com/sauravpanda/bonsai/internal/git"
	"github.com/spf13/cobra"
)

var rmCmd = &cobra.Command{
	Use:   "rm <n> [n...]",
	Short: "Delete worktrees by number",
	Long: `Delete one or more worktrees by the numbers shown in 'bonsai list'.

Examples:
  bonsai rm 2          delete worktree #2
  bonsai rm 1 3 5      delete worktrees #1, #3, and #5
  bonsai rm --dry-run 2  show what would be deleted`,
	Args: cobra.MinimumNArgs(1),
	RunE: runRm,
}

func init() {
	rootCmd.AddCommand(rmCmd)
	rmCmd.Flags().Bool("force", false, "remove even if branch has unpushed commits")
	rmCmd.Flags().Bool("dry-run", false, "show what would be removed without doing it")
}

var rmDimStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

func runRm(cmd *cobra.Command, args []string) error {
	force, _ := cmd.Flags().GetBool("force")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	worktrees, err := git.List()
	if err != nil {
		return err
	}

	added := AddedWorktrees(worktrees)
	if len(added) == 0 {
		fmt.Println("No added worktrees.")
		return nil
	}

	// Parse requested indices (1-based).
	var targets []*git.Worktree
	seen := map[int]bool{}
	for _, a := range args {
		n, err := strconv.Atoi(strings.TrimSpace(a))
		if err != nil || n < 1 || n > len(added) {
			return fmt.Errorf("invalid worktree number %q (valid range: 1–%d)", a, len(added))
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		targets = append(targets, added[n-1])
	}

	// Enrich just the targets we need.
	for _, wt := range targets {
		git.Enrich(wt, cfg.DefaultBase, cfg.DefaultRemote)
	}

	// Check for unpushed commits.
	var blocked []*git.Worktree
	for _, wt := range targets {
		if wt.HasUnpushed && !force {
			blocked = append(blocked, wt)
		}
	}
	if len(blocked) > 0 {
		fmt.Fprintf(os.Stderr, "%s the following worktrees have unpushed commits:\n",
			warnStyle.Render("⚠"))
		for _, wt := range blocked {
			fmt.Fprintf(os.Stderr, "  • %s  %s\n",
				lipgloss.NewStyle().Bold(true).Render(wt.Branch),
				rmDimStyle.Render(wt.Path))
		}
		fmt.Fprintln(os.Stderr, "\nUse --force to remove them anyway.")
		return fmt.Errorf("aborted")
	}

	// Execute (or preview).
	root, _ := git.MainRoot()
	var hadErr bool
	for _, wt := range targets {
		short := git.ShortenPath(wt.Path, root)
		if dryRun {
			fmt.Printf("  [dry-run] remove  %s  %s\n",
				lipgloss.NewStyle().Bold(true).Render(wt.Branch),
				rmDimStyle.Render(short))
			continue
		}
		fmt.Printf("  removing  %-28s  %s … ",
			wt.Branch, rmDimStyle.Render(short))
		if err := git.Remove(wt.Path, force); err != nil {
			fmt.Printf("%s\n", warnStyle.Render("failed: "+err.Error()))
			hadErr = true
		} else {
			fmt.Println(okStyle.Render("done"))
		}
	}
	if hadErr {
		return fmt.Errorf("one or more worktrees could not be removed")
	}
	return nil
}
