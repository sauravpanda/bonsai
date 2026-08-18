package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/sauravpanda/bonsai/internal/config"
	"github.com/sauravpanda/bonsai/internal/git"
	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Rebase all worktrees from the base branch",
	Long: `Sync updates all non-main worktrees by rebasing them onto the base branch
(default: main). Worktrees with uncommitted changes are skipped with a warning.

Flags:
  --merge    use git merge instead of git rebase
  --dry-run  show what would be done without running it

Examples:
  bonsai sync
  bonsai sync --merge
  bonsai sync --dry-run`,
	RunE: runSync,
}

func init() {
	rootCmd.AddCommand(syncCmd)
	syncCmd.Flags().Bool("merge", false, "use git merge instead of git rebase")
	syncCmd.Flags().Bool("dry-run", false, "show what would be synced without running")
}

func runSync(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	useMerge, _ := cmd.Flags().GetBool("merge")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	worktrees, err := git.List()
	if err != nil {
		return err
	}

	base := cfg.DefaultBase
	remote := cfg.DefaultRemote

	// Fetch the base branch first.
	if !dryRun {
		fmt.Printf("  fetching %s/%s …\n", remote, base)
		fetchCmd := exec.Command("git", "fetch", remote, base)
		fetchCmd.Stdout = os.Stdout
		fetchCmd.Stderr = os.Stderr
		if err := fetchCmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "  warning: fetch failed: %v\n", err)
		}
	}

	verb := "rebase"
	if useMerge {
		verb = "merge"
	}
	baseRef := remote + "/" + base

	var synced, skipped, failed int

	for _, wt := range worktrees {
		if wt.IsMain {
			continue
		}

		branch := wt.Branch
		if branch == "" || branch == "(detached)" {
			continue
		}

		// Check for every kind of uncommitted work, not only modified files.
		dirty, staged, untracked, known := git.WorkingTreeStatus(wt.Path)
		if !known || dirty+staged+untracked > 0 {
			detail := fmt.Sprintf("%d modified, %d staged, %d untracked", dirty, staged, untracked)
			if !known {
				detail = "working tree status unavailable"
			}
			fmt.Printf("  %-28s  %s\n",
				truncate(branch, 28),
				warnStyle.Render("skipped — "+detail),
			)
			skipped++
			continue
		}

		if dryRun {
			fmt.Printf("  %-28s  %s\n",
				truncate(branch, 28),
				dimStyle.Render(fmt.Sprintf("[dry-run] would %s from %s", verb, baseRef)),
			)
			continue
		}

		fmt.Printf("  %-28s  %s … ", truncate(branch, 28), verb)

		var syncArgs []string
		if useMerge {
			syncArgs = []string{"-C", wt.Path, "merge", baseRef, "--no-edit"}
		} else {
			syncArgs = []string{"-C", wt.Path, "rebase", baseRef}
		}

		out, err := exec.Command("git", syncArgs...).CombinedOutput()
		outStr := strings.TrimSpace(string(out))

		if err != nil {
			fmt.Println(errStyle.Render("failed"))
			if outStr != "" {
				for _, line := range strings.Split(outStr, "\n") {
					fmt.Printf("    %s\n", line)
				}
			}
			// Abort on conflict so the worktree is not left in a broken state.
			if abortErr := abortFailedSync(wt.Path, useMerge); abortErr != nil {
				fmt.Fprintf(os.Stderr, "    %s\n",
					warnStyle.Render("warning: could not abort cleanly: "+abortErr.Error()))
			}
			failed++
		} else {
			fmt.Println(okStyle.Render("ok"))
			synced++
		}
	}

	if !dryRun {
		fmt.Printf("\n%s synced, %s skipped, %s failed\n",
			okStyle.Render(fmt.Sprint(synced)),
			dimStyle.Render(fmt.Sprint(skipped)),
			errStyle.Render(fmt.Sprint(failed)),
		)
		if failed > 0 {
			return fmt.Errorf("%d worktree(s) failed to sync", failed)
		}
	}

	return nil
}

func abortFailedSync(path string, useMerge bool) error {
	operation := "rebase"
	if useMerge {
		operation = "merge"
	}
	out, err := exec.Command("git", "-C", path, operation, "--abort").CombinedOutput()
	if err == nil {
		return nil
	}
	detail := strings.TrimSpace(string(out))
	if detail == "" {
		return fmt.Errorf("git %s --abort: %w", operation, err)
	}
	return fmt.Errorf("git %s --abort: %w: %s", operation, err, detail)
}
