package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/sauravpanda/bonsai/internal/config"
	"github.com/sauravpanda/bonsai/internal/git"
	"github.com/sauravpanda/bonsai/internal/github"
	"github.com/sauravpanda/bonsai/internal/tui"
	"github.com/spf13/cobra"
)

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Interactively delete merged/stale worktrees",
	Long: `Launch an interactive TUI to select and delete worktrees.

Candidates are worktrees with:
  - A merged GitHub PR
  - No activity for longer than the stale threshold (default 14 days)
  - No unpushed commits

Worktrees with unpushed commits can still be selected but require
explicit confirmation.`,
	RunE: runClean,
}

func init() {
	rootCmd.AddCommand(cleanCmd)
	cleanCmd.Flags().IntP("stale", "s", 0, "override stale threshold in days")
	cleanCmd.Flags().Bool("force", false, "force removal even if worktree is dirty")
	cleanCmd.Flags().Bool("all", false, "show all worktrees, not just candidates")
	cleanCmd.Flags().Bool("offline", false, "skip GitHub PR status lookup (faster)")
}

func runClean(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if cmd.Flags().Changed("stale") {
		staleOverride, _ := cmd.Flags().GetInt("stale")
		cfg.StaleThresholdDays = staleOverride
	}
	force, _ := cmd.Flags().GetBool("force")
	showAll, _ := cmd.Flags().GetBool("all")
	offline, _ := cmd.Flags().GetBool("offline")

	worktrees, err := git.List()
	if err != nil {
		return err
	}

	ghOK := !offline && github.IsAvailable()
	root, _ := git.MainRoot()

	spin := tui.Start("scanning worktrees…")
	for _, wt := range worktrees {
		git.Enrich(wt, cfg.DefaultBase, cfg.DefaultRemote)
		if ghOK && !wt.IsMain && !wt.IsDetached && wt.Branch != "" {
			spin.UpdateMsg(fmt.Sprintf("checking PR status for %s…", wt.Branch))
			pr, err := github.GetPR(wt.Branch)
			if err == nil {
				wt.PRStatus = strings.ToLower(pr.State)
				wt.PRURL = pr.URL
			} else {
				wt.PRStatus = "none"
			}
		}
	}
	spin.Stop()

	staleDur := float64(cfg.StaleThresholdDays) * 24 * 3600e9 // nanoseconds

	var items []tui.Item
	for _, wt := range worktrees {
		if wt.IsMain {
			continue
		}

		reasons := candidateReasons(wt, staleDur)
		if !showAll && len(reasons) == 0 {
			continue
		}

		path := git.ShortenPath(wt.Path, root)
		label := fmt.Sprintf("%-28s  %s", truncate(wt.Branch, 28), truncate(path, 40))

		desc := ""
		if len(reasons) > 0 {
			desc = strings.Join(reasons, " · ")
		} else {
			desc = "no specific reason (--all flag)"
		}
		if wt.HasUnpushed {
			desc += " · ⚠ unpushed"
		}

		items = append(items, tui.Item{
			ID:          wt.Path,
			Label:       label,
			Desc:        desc,
			HasUnpushed: wt.HasUnpushed,
		})
	}

	if len(items) == 0 {
		fmt.Println("No worktree candidates found. Use --all to show all worktrees.")
		return nil
	}

	result, err := tui.Run("bonsai clean — select worktrees to delete", items)
	if err != nil {
		return err
	}

	if !result.Confirmed {
		fmt.Println("Aborted.")
		return nil
	}

	var toDelete []tui.Item
	for _, it := range result.Items {
		if it.Selected {
			toDelete = append(toDelete, it)
		}
	}

	if len(toDelete) == 0 {
		fmt.Println("Nothing selected.")
		return nil
	}

	// Warn about unpushed commits
	var withUnpushed []tui.Item
	for _, it := range toDelete {
		if it.HasUnpushed {
			withUnpushed = append(withUnpushed, it)
		}
	}
	if len(withUnpushed) > 0 {
		fmt.Printf("\n%s The following worktrees have unpushed commits:\n", warnStyle.Render("⚠"))
		for _, it := range withUnpushed {
			fmt.Printf("  • %s\n", it.Label)
		}
		if !confirm("\nDelete anyway? [y/N] ") {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// Execute deletions
	ok, failed := 0, 0
	for _, it := range toDelete {
		fmt.Printf("  removing %s ... ", it.ID)
		if err := git.Remove(it.ID, force || it.HasUnpushed); err != nil {
			fmt.Printf("error: %v\n", err)
			failed++
		} else {
			fmt.Println("done")
			ok++
		}
	}

	fmt.Printf("\n%d removed", ok)
	if failed > 0 {
		fmt.Printf(", %d failed (try --force)", failed)
	}
	fmt.Println()
	return nil
}

// candidateReasons returns the reasons a worktree is a deletion candidate.
func candidateReasons(wt *git.Worktree, staleDur float64) []string {
	var reasons []string
	if wt.PRStatus == "merged" {
		reasons = append(reasons, "merged PR")
	}
	if wt.Age > 0 && float64(wt.Age) > staleDur {
		reasons = append(reasons, fmt.Sprintf("stale (%s)", git.FormatAge(wt.Age)))
	}
	if !wt.HasUnpushed && wt.PRStatus != "open" && wt.PRStatus != "merged" {
		reasons = append(reasons, "no unpushed commits")
	}
	return reasons
}

func confirm(prompt string) bool {
	fmt.Print(prompt)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	return strings.ToLower(strings.TrimSpace(scanner.Text())) == "y"
}

// confirmOrQuit reads a [y/N/q] prompt and returns (yes, quit).
// "y" → (true, false), "q" → (false, true), anything else → (false, false).
func confirmOrQuit(prompt string) (yes bool, quit bool) {
	fmt.Print(prompt)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	switch strings.ToLower(strings.TrimSpace(scanner.Text())) {
	case "y":
		return true, false
	case "q":
		return false, true
	default:
		return false, false
	}
}
