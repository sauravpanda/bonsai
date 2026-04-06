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
	"golang.org/x/sync/errgroup"
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
	pruneCmd.Flags().Bool("offline", false, "skip GitHub PR status lookup (faster)")
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
	offline, _ := cmd.Flags().GetBool("offline")

	worktrees, err := git.List()
	if err != nil {
		return err
	}

	ghOK := !offline && github.IsAvailable()
	root, _ := git.MainRoot()
	staleDur := float64(cfg.StaleThresholdDays) * 24 * 3600e9

	// Collect non-main worktrees and enrich them in parallel.
	var added []*git.Worktree
	for _, wt := range worktrees {
		if !wt.IsMain {
			added = append(added, wt)
		}
	}

	spin := tui.Start(fmt.Sprintf("enriching %d worktree(s) in parallel…", len(added)))
	var g errgroup.Group
	for _, wt := range added {
		wt := wt
		g.Go(func() error {
			git.Enrich(wt, cfg.DefaultBase, cfg.DefaultRemote)
			if ghOK && !wt.IsDetached && wt.Branch != "" {
				pr, err := github.GetPR(wt.Branch)
				if err == nil {
					wt.PRStatus = strings.ToLower(pr.State)
					wt.PRURL = pr.URL
				} else {
					wt.PRStatus = "none"
				}
			}
			return nil
		})
	}
	g.Wait() //nolint:errcheck — goroutines always return nil
	spin.Stop()

	var candidates []pruneCandidate
	for _, wt := range added {
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
			yes, quit := confirmOrQuit("  Delete? [y/N/q] ")
			if quit {
				fmt.Println("  quitting")
				break
			}
			shouldDelete = yes
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
