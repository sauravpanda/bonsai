package cmd

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/sauravpanda/bonsai/internal/config"
	"github.com/sauravpanda/bonsai/internal/git"
	"github.com/sauravpanda/bonsai/internal/github"
	"github.com/sauravpanda/bonsai/internal/tui"
	"github.com/spf13/cobra"
)

var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Suggest and delete stale worktrees (non-interactive)",
	Long: `Prune analyzes all worktrees and suggests deletions with reasoning.

Without --dry-run, each candidate is shown with its reason and you are
asked to confirm before deletion. Use --yes to auto-confirm all.`,
	RunE: runPrune,
}

func init() {
	rootCmd.AddCommand(pruneCmd)
	pruneCmd.Flags().Bool("dry-run", false, "print candidates without deleting")
	pruneCmd.Flags().BoolP("yes", "y", false, "auto-confirm all deletions (no prompts)")
	pruneCmd.Flags().IntP("stale", "s", 0, "override stale threshold in days")
	pruneCmd.Flags().Bool("force", false, "force removal even if worktree is dirty")
}

var (
	reasonStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	warnStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	boldStyle   = lipgloss.NewStyle().Bold(true)
)

type pruneCandidate struct {
	wt      *git.Worktree
	reasons []string
	path    string // shortened
}

func runPrune(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if cmd.Flags().Changed("stale") {
		staleOverride, _ := cmd.Flags().GetInt("stale")
		cfg.StaleThresholdDays = staleOverride
	}
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	autoYes, _ := cmd.Flags().GetBool("yes")
	force, _ := cmd.Flags().GetBool("force")

	worktrees, err := git.List()
	if err != nil {
		return err
	}

	ghOK := github.IsAvailable()
	root, _ := git.MainRoot()
	staleDur := float64(cfg.StaleThresholdDays) * 24 * 3600e9

	spin := tui.Start("scanning worktrees…")
	var candidates []pruneCandidate
	for _, wt := range worktrees {
		if wt.IsMain {
			continue
		}
		git.Enrich(wt, cfg.DefaultBase, cfg.DefaultRemote)
		if ghOK && !wt.IsDetached && wt.Branch != "" {
			spin.UpdateMsg(fmt.Sprintf("checking PR status for %s…", wt.Branch))
			pr, err := github.GetPR(wt.Branch)
			if err == nil {
				wt.PRStatus = strings.ToLower(pr.State)
				wt.PRURL = pr.URL
			} else {
				wt.PRStatus = "none"
			}
		}

		reasons := candidateReasons(wt, staleDur)
		if len(reasons) == 0 {
			continue
		}
		candidates = append(candidates, pruneCandidate{
			wt:      wt,
			reasons: reasons,
			path:    git.ShortenPath(wt.Path, root),
		})
	}
	spin.Stop()

	if len(candidates) == 0 {
		fmt.Println(okStyle.Render("✓") + "  No worktrees to prune.")
		return nil
	}

	fmt.Printf("Found %s prune candidate(s):\n\n", boldStyle.Render(fmt.Sprint(len(candidates))))

	if dryRun {
		for _, c := range candidates {
			printCandidate(c)
		}
		fmt.Printf("\n%s (dry-run, no changes made)\n",
			reasonStyle.Render(fmt.Sprintf("Would delete %d worktree(s)", len(candidates))))
		return nil
	}

	// Interactive or auto-confirm deletion
	deleted, skipped := 0, 0
	for _, c := range candidates {
		printCandidate(c)

		if c.wt.HasUnpushed && !force {
			fmt.Println("  " + warnStyle.Render("⚠  This worktree has unpushed commits."))
		}

		shouldDelete := autoYes
		if !autoYes {
			shouldDelete = confirm("  Delete? [y/N/q] ")
			// Allow 'q' to quit early — handled by confirm returning false on 'q'
		}

		if shouldDelete {
			fmt.Printf("  removing ... ")
			// If the worktree has unpushed commits the user already saw the warning
		// and confirmed deletion — treat that as implicit force regardless of
		// whether --yes or --force was passed.
		useForce := force || c.wt.HasUnpushed
			if err := git.Remove(c.wt.Path, useForce); err != nil {
				fmt.Printf("error: %v\n", err)
			} else {
				fmt.Println(okStyle.Render("done"))
				deleted++
			}
		} else {
			fmt.Println("  skipped")
			skipped++
		}
		fmt.Println()
	}

	fmt.Printf("%s deleted, %s skipped\n",
		okStyle.Render(fmt.Sprint(deleted)),
		reasonStyle.Render(fmt.Sprint(skipped)),
	)
	return nil
}

func printCandidate(c pruneCandidate) {
	fmt.Printf("  %s  %s\n",
		boldStyle.Render(c.wt.Branch),
		reasonStyle.Render(c.path),
	)
	fmt.Printf("  reason: %s\n", strings.Join(c.reasons, " · "))
	if c.wt.LastCommit != "" {
		fmt.Printf("  last:   %s (%s ago)\n", truncate(c.wt.LastCommit, 60), git.FormatAge(c.wt.Age))
	}
}
