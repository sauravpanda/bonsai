package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/sauravpanda/bonsai/internal/config"
	"github.com/sauravpanda/bonsai/internal/git"
	"github.com/spf13/cobra"
)

var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Safely plan and remove merged or stale worktrees",
	Long: `Prune classifies merged or inactive worktrees as safe, review, or protected.

Automatic deletion only removes safe worktrees. Dirty files, untracked files,
unpushed commits, open PRs, locked worktrees, the current worktree, and unknown
states are protected.

Use --json to create a machine-readable, short-lived cleanup plan, then apply
that exact plan with --apply <plan-id> --yes. Use --claude to restrict analysis
to worktrees under .claude/worktrees/, and --global to discover repositories
across bounded development roots.`,
	RunE: runPrune,
}

func init() {
	rootCmd.AddCommand(pruneCmd)
	pruneCmd.Flags().Bool("dry-run", false, "show classifications without deleting")
	pruneCmd.Flags().BoolP("yes", "y", false, "remove all safe worktrees without prompts")
	pruneCmd.Flags().IntP("stale", "s", 0, "override stale threshold in days")
	pruneCmd.Flags().Bool("force", false, "allow interactive removal of review/protected worktrees")
	pruneCmd.Flags().Bool("offline", false, "skip GitHub PR status lookup")
	pruneCmd.Flags().Bool("claude", false, "only inspect worktrees under .claude/worktrees")
	pruneCmd.Flags().Bool("global", false, "discover and inspect worktrees across development roots")
	pruneCmd.Flags().StringSlice("root", nil, "global scan root (repeatable; defaults to common development directories)")
	pruneCmd.Flags().Bool("json", false, "emit a saved cleanup plan or apply result as JSON")
	pruneCmd.Flags().String("apply", "", "apply a saved cleanup plan by ID (requires --yes)")
}

func runPrune(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	hasStaleOverride := cmd.Flags().Changed("stale")
	if hasStaleOverride {
		staleOverride, _ := cmd.Flags().GetInt("stale")
		cfg.StaleThresholdDays = staleOverride
	}

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	autoYes, _ := cmd.Flags().GetBool("yes")
	force, _ := cmd.Flags().GetBool("force")
	offline, _ := cmd.Flags().GetBool("offline")
	claudeOnly, _ := cmd.Flags().GetBool("claude")
	global, _ := cmd.Flags().GetBool("global")
	scanRoots, _ := cmd.Flags().GetStringSlice("root")
	asJSON, _ := cmd.Flags().GetBool("json")
	applyID, _ := cmd.Flags().GetString("apply")
	if len(scanRoots) > 0 && !global {
		return fmt.Errorf("--root requires --global")
	}

	if autoYes && force {
		return fmt.Errorf("--yes cannot be combined with --force; automatic cleanup is safe-only")
	}
	if applyID != "" {
		if !autoYes {
			return fmt.Errorf("--apply requires --yes after the cleanup plan has been approved")
		}
		if dryRun || force {
			return fmt.Errorf("--apply cannot be combined with --dry-run or --force")
		}
		result, err := applyCleanupPlan(applyID)
		if err != nil {
			return err
		}
		if asJSON {
			return writeJSON(result)
		}
		printApplyResult(result)
		return nil
	}

	plan, err := buildCleanupPlan(cfg, cleanupPlanOptions{
		ClaudeOnly:       claudeOnly,
		Offline:          offline,
		Progress:         !asJSON,
		Global:           global,
		ScanRoots:        scanRoots,
		StaleOverride:    cfg.StaleThresholdDays,
		HasStaleOverride: hasStaleOverride,
	})
	if err != nil {
		return err
	}

	if asJSON {
		if err := saveCleanupPlan(plan); err != nil {
			return fmt.Errorf("save cleanup plan: %w", err)
		}
		return writeJSON(plan)
	}

	printCleanupPlan(plan)
	if dryRun {
		fmt.Println(dimStyle.Render("\n  dry-run: no changes made"))
		return nil
	}
	if len(plan.Safe) == 0 && (!force || len(plan.Review)+len(plan.Protected) == 0) {
		return nil
	}

	if autoYes {
		if err := saveCleanupPlan(plan); err != nil {
			return fmt.Errorf("save cleanup plan: %w", err)
		}
		result, err := applyCleanupPlan(plan.PlanID)
		if err != nil {
			return err
		}
		printApplyResult(result)
		return nil
	}

	return runInteractivePrune(plan, force)
}

func writeJSON(value any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func printCleanupPlan(plan *cleanupPlan) {
	fmt.Printf("\n  %s  analyzed %d worktree(s) across %d repo(s) · scope: %s\n",
		okStyle.Render("🌱"), plan.Summary.Scanned, plan.Summary.Repositories, plan.Scope)
	for _, warning := range plan.Warnings {
		fmt.Printf("  %s %s\n", warnStyle.Render("⚠"), warning)
	}
	printCleanupSection("SAFE TO PRUNE", plan.Safe, okStyle)
	printCleanupSection("NEEDS REVIEW", plan.Review, warnStyle)
	printCleanupSection("PROTECTED", plan.Protected, errStyle)

	if len(plan.Safe) == 0 {
		fmt.Printf("\n  %s No worktrees are currently safe to prune.\n", okStyle.Render("✓"))
		return
	}
	fmt.Printf("\n  %s safe worktree(s)  ·  %s reclaimable  ·  local branches included\n",
		boldStyle.Render(fmt.Sprint(len(plan.Safe))),
		okBoldStyle.Render(formatBytes(plan.Summary.ReclaimableBytes)),
	)
}

func printCleanupSection(title string, items []cleanupItem, style interface{ Render(...string) string }) {
	if len(items) == 0 {
		return
	}
	fmt.Printf("\n  %s (%d)\n", style.Render(title), len(items))
	for _, item := range items {
		label := item.Branch
		if item.RepositoryName != "" {
			label = item.RepositoryName + "/" + item.Branch
		}
		fmt.Printf("  %s %-36s %8s  %s\n",
			stateGlyph(item.State), truncate(label, 36), formatBytes(item.DiskBytes),
			dimStyle.Render(strings.Join(item.Reasons, " · ")),
		)
	}
}

func stateGlyph(state cleanupState) string {
	switch state {
	case cleanupSafe:
		return okStyle.Render("✓")
	case cleanupReview:
		return warnStyle.Render("?")
	default:
		return errStyle.Render("●")
	}
}

func runInteractivePrune(plan *cleanupPlan, force bool) error {
	var targets []cleanupItem
	targets = append(targets, plan.Safe...)
	if force {
		targets = append(targets, plan.Review...)
		targets = append(targets, plan.Protected...)
	}
	if len(targets) == 0 {
		return nil
	}

	var result cleanupApplyResult
	skipped := 0
	for _, item := range targets {
		fmt.Printf("\n  %s  %s\n", boldStyle.Render(item.Branch), dimStyle.Render(item.ShortPath))
		fmt.Printf("  reason: %s\n", strings.Join(item.Reasons, " · "))
		if item.State != cleanupSafe {
			fmt.Println("  " + warnStyle.Render("⚠ explicit --force allows this protected deletion"))
		}
		yes, quit := confirmOrQuit("  Delete? [y/N/q] ")
		if quit {
			break
		}
		if !yes {
			skipped++
			continue
		}
		fmt.Print("  removing ... ")
		if item.State == cleanupSafe {
			if err := validateCleanupItem(item); err != nil {
				fmt.Printf("skipped: %v\n", err)
				skipped++
				continue
			}
		}
		if err := git.RemoveAt(item.RepositoryRoot, item.Path, item.State != cleanupSafe); err != nil {
			fmt.Printf("error: %v\n", err)
			continue
		}
		fmt.Println(okStyle.Render("done"))
		result.Removed++
		result.ReclaimedBytes += item.DiskBytes
		if item.State == cleanupSafe && item.DeleteBranch {
			if err := git.DeleteBranchAt(item.RepositoryRoot, item.Branch, true); err != nil {
				result.BranchDeleteFail = append(result.BranchDeleteFail, item.Branch+": "+err.Error())
			} else {
				result.BranchesDeleted++
			}
		}
	}
	printApplyResult(result)
	if skipped > 0 {
		fmt.Printf("  %d skipped\n", skipped)
	}
	return nil
}

func printApplyResult(result cleanupApplyResult) {
	fmt.Printf("\n  %s %d removed · %d local branch(es) pruned · %s reclaimed\n",
		okBoldStyle.Render("✓"), result.Removed, result.BranchesDeleted,
		okBoldStyle.Render(formatBytes(result.ReclaimedBytes)),
	)
	for _, failure := range result.BranchDeleteFail {
		fmt.Printf("  %s branch cleanup failed: %s\n", warnStyle.Render("⚠"), failure)
	}
}
