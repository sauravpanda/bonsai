package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/sauravpanda/bonsai/internal/config"
	"github.com/sauravpanda/bonsai/internal/git"
	"github.com/sauravpanda/bonsai/internal/tui"
	"github.com/spf13/cobra"
)

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Interactively delete merged/stale worktrees",
	Long: `Launch an interactive TUI to select and delete worktrees.

Candidates are worktrees with a merged GitHub PR or no activity beyond the
stale threshold. Every candidate uses the same safe, review, and protected
classification as bonsai prune. Protected worktrees require explicit --force.

Use --global to discover repositories across bounded development roots, and
--all to include recent worktrees that are not cleanup candidates.`,
	RunE: runClean,
}

func init() {
	rootCmd.AddCommand(cleanCmd)
	cleanCmd.Flags().IntP("stale", "s", 0, "override stale threshold in days")
	cleanCmd.Flags().Bool("force", false, "allow selection of review/protected worktrees")
	cleanCmd.Flags().Bool("all", false, "show all worktrees, not just cleanup candidates")
	cleanCmd.Flags().Bool("offline", false, "skip GitHub PR status lookup")
	cleanCmd.Flags().Bool("claude", false, "only inspect worktrees under .claude/worktrees")
	cleanCmd.Flags().Bool("global", false, "discover and inspect worktrees across development roots")
	cleanCmd.Flags().StringSlice("root", nil, "global scan root (repeatable; defaults to common development directories)")
}

func runClean(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	hasStaleOverride := cmd.Flags().Changed("stale")
	if hasStaleOverride {
		staleOverride, _ := cmd.Flags().GetInt("stale")
		cfg.StaleThresholdDays = staleOverride
	}
	force, _ := cmd.Flags().GetBool("force")
	showAll, _ := cmd.Flags().GetBool("all")
	offline, _ := cmd.Flags().GetBool("offline")
	claudeOnly, _ := cmd.Flags().GetBool("claude")
	global, _ := cmd.Flags().GetBool("global")
	scanRoots, _ := cmd.Flags().GetStringSlice("root")
	if len(scanRoots) > 0 && !global {
		return fmt.Errorf("--root requires --global")
	}

	plan, err := buildCleanupPlan(cfg, cleanupPlanOptions{
		ClaudeOnly:       claudeOnly,
		Offline:          offline,
		Progress:         true,
		Global:           global,
		ScanRoots:        scanRoots,
		IncludeAll:       showAll,
		StaleOverride:    cfg.StaleThresholdDays,
		HasStaleOverride: hasStaleOverride,
	})
	if err != nil {
		return err
	}
	for _, warning := range plan.Warnings {
		fmt.Printf("  %s %s\n", warnStyle.Render("⚠"), warning)
	}
	if plan.Summary.Scanned == 0 {
		if global {
			fmt.Printf("No added worktrees found across %d discovered repositories.\n", plan.Summary.Repositories)
		} else {
			fmt.Println("No added worktrees found in this repository. Use --global to scan your development roots.")
		}
		return nil
	}

	itemsByPath := make(map[string]cleanupItem)
	var pickerItems []tui.Item
	addItems := func(items []cleanupItem) {
		for _, item := range items {
			label := fmt.Sprintf("%-36s  %s",
				truncate(item.RepositoryName+"/"+item.Branch, 36),
				truncateLeft(item.ShortPath, 40),
			)
			pickerItems = append(pickerItems, tui.Item{
				ID:        item.Path,
				Label:     label,
				Desc:      string(item.State) + " · " + strings.Join(item.Reasons, " · "),
				Protected: item.State != cleanupSafe,
			})
			itemsByPath[item.Path] = item
		}
	}
	addItems(plan.Safe)
	addItems(plan.Review)
	addItems(plan.Protected)

	if len(pickerItems) == 0 {
		fmt.Printf("No merged or inactive candidates found across %d added worktree(s). Use --all to review them.\n", plan.Summary.Scanned)
		return nil
	}

	result, err := tui.RunWithOptions(
		"bonsai clean — select worktrees to delete",
		pickerItems,
		tui.PickerOptions{AllowProtected: force},
	)
	if err != nil {
		return err
	}
	if !result.Confirmed {
		fmt.Println("Aborted.")
		return nil
	}

	var selected []tui.Item
	for _, item := range result.Items {
		if item.Selected {
			selected = append(selected, item)
		}
	}
	if len(selected) == 0 {
		fmt.Println("Nothing selected.")
		return nil
	}

	var protected []tui.Item
	for _, item := range selected {
		if item.Protected {
			protected = append(protected, item)
		}
	}
	if len(protected) > 0 && !force {
		fmt.Printf("\n%s The following worktrees are review/protected:\n", warnStyle.Render("⚠"))
		for _, item := range protected {
			fmt.Printf("  • %s\n", item.Label)
		}
		return fmt.Errorf("review/protected worktrees require the explicit --force flag")
	}

	cleanupResult := cleanupApplyResult{}
	failed := 0
	for _, selectedItem := range selected {
		planned := itemsByPath[selectedItem.ID]
		fmt.Printf("  removing %s/%s ... ", planned.RepositoryName, planned.Branch)
		if planned.State == cleanupSafe {
			if err := validateCleanupItem(planned); err != nil {
				fmt.Printf("skipped: %v\n", err)
				failed++
				continue
			}
		}
		if err := git.RemoveAt(planned.RepositoryRoot, planned.Path, planned.State != cleanupSafe); err != nil {
			fmt.Printf("error: %v\n", err)
			failed++
			continue
		}
		fmt.Println(okStyle.Render("done"))
		cleanupResult.Removed++
		cleanupResult.ReclaimedBytes += planned.DiskBytes
		if planned.DeleteBranch {
			if err := git.DeleteBranchAt(planned.RepositoryRoot, planned.Branch, true); err != nil {
				cleanupResult.BranchDeleteFail = append(cleanupResult.BranchDeleteFail, planned.RepositoryName+"/"+planned.Branch+": "+err.Error())
			} else {
				cleanupResult.BranchesDeleted++
			}
		}
	}

	printApplyResult(cleanupResult)
	if failed > 0 {
		fmt.Printf("  %d failed\n", failed)
	}
	return nil
}

// nsPerDay is the number of nanoseconds in a day, used to convert
// StaleThresholdDays (user-facing config) to a time.Duration-compatible value.
const nsPerDay = float64(24 * 3600 * 1e9)

// staleDuration converts a day count into the ns-scale float the worktree
// age comparison expects.
func staleDuration(days int) float64 { return float64(days) * nsPerDay }

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
