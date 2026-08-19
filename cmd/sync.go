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
  bonsai sync --dry-run
  bonsai sync --dry-run --json`,
	RunE: runSync,
}

func init() {
	rootCmd.AddCommand(syncCmd)
	syncCmd.Flags().Bool("merge", false, "use git merge instead of git rebase")
	syncCmd.Flags().Bool("dry-run", false, "show what would be synced without running")
	syncCmd.Flags().Bool("json", false, "emit per-worktree results as JSON to stdout")
}

type syncJSONResult struct {
	Path   string `json:"path"`
	Branch string `json:"branch"`
	Result string `json:"result"`
	Reason string `json:"reason,omitempty"`
	Error  string `json:"error,omitempty"`
}

type syncJSONSummary struct {
	Synced  int `json:"synced"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
}

// syncJSONOutput is the stable machine-readable shape emitted by
// `bonsai sync --json`.
type syncJSONOutput struct {
	Base       string           `json:"base"`
	Remote     string           `json:"remote"`
	Strategy   string           `json:"strategy"`
	DryRun     bool             `json:"dry_run"`
	FetchError string           `json:"fetch_error,omitempty"`
	Results    []syncJSONResult `json:"results"`
	Summary    syncJSONSummary  `json:"summary"`
}

func runSync(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	useMerge, _ := cmd.Flags().GetBool("merge")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	asJSON, _ := cmd.Flags().GetBool("json")

	worktrees, err := git.List()
	if err != nil {
		return err
	}

	base := cfg.DefaultBase
	remote := cfg.DefaultRemote
	verb := "rebase"
	if useMerge {
		verb = "merge"
	}
	jsonOutput := syncJSONOutput{
		Base:     base,
		Remote:   remote,
		Strategy: verb,
		DryRun:   dryRun,
		Results:  make([]syncJSONResult, 0),
	}

	// Fetch the base branch first.
	if !dryRun {
		fetchCmd := exec.Command("git", "fetch", remote, base)
		if asJSON {
			out, fetchErr := fetchCmd.CombinedOutput()
			if fetchErr != nil {
				jsonOutput.FetchError = commandFailure(fetchErr, out)
			}
		} else {
			fmt.Printf("  fetching %s/%s …\n", remote, base)
			fetchCmd.Stdout = os.Stdout
			fetchCmd.Stderr = os.Stderr
			if err := fetchCmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "  warning: fetch failed: %v\n", err)
			}
		}
	}

	baseRef := remote + "/" + base

	var synced, skipped, failed int

	for _, wt := range worktrees {
		if wt.IsMain {
			continue
		}

		branch := wt.Branch
		if branch == "" || branch == "(detached)" {
			if asJSON {
				jsonOutput.Results = append(jsonOutput.Results, syncJSONResult{
					Path: wt.Path, Branch: branch, Result: "skipped", Reason: "detached worktree",
				})
				skipped++
			}
			continue
		}

		// Check for every kind of uncommitted work, not only modified files.
		dirty, staged, untracked, known := git.WorkingTreeStatus(wt.Path)
		if !known || dirty+staged+untracked > 0 {
			detail := fmt.Sprintf("%d modified, %d staged, %d untracked", dirty, staged, untracked)
			if !known {
				detail = "working tree status unavailable"
			}
			if asJSON {
				jsonOutput.Results = append(jsonOutput.Results, syncJSONResult{
					Path: wt.Path, Branch: branch, Result: "skipped", Reason: detail,
				})
			} else {
				fmt.Printf("  %-28s  %s\n",
					truncate(branch, 28),
					warnStyle.Render("skipped — "+detail),
				)
			}
			skipped++
			continue
		}

		if dryRun {
			detail := fmt.Sprintf("dry-run: would %s from %s", verb, baseRef)
			if asJSON {
				jsonOutput.Results = append(jsonOutput.Results, syncJSONResult{
					Path: wt.Path, Branch: branch, Result: "skipped", Reason: detail,
				})
				skipped++
			} else {
				fmt.Printf("  %-28s  %s\n",
					truncate(branch, 28),
					dimStyle.Render(fmt.Sprintf("[dry-run] would %s from %s", verb, baseRef)),
				)
			}
			continue
		}

		if !asJSON {
			fmt.Printf("  %-28s  %s … ", truncate(branch, 28), verb)
		}

		var syncArgs []string
		if useMerge {
			syncArgs = []string{"-C", wt.Path, "merge", baseRef, "--no-edit"}
		} else {
			syncArgs = []string{"-C", wt.Path, "rebase", baseRef}
		}

		out, err := exec.Command("git", syncArgs...).CombinedOutput()
		outStr := strings.TrimSpace(string(out))

		if err != nil {
			failure := commandFailure(err, out)
			if !asJSON {
				fmt.Println(errStyle.Render("failed"))
				if outStr != "" {
					for _, line := range strings.Split(outStr, "\n") {
						fmt.Printf("    %s\n", line)
					}
				}
			}
			// Abort on conflict so the worktree is not left in a broken state.
			if abortErr := abortFailedSync(wt.Path, useMerge); abortErr != nil {
				failure += "; could not abort cleanly: " + abortErr.Error()
				if !asJSON {
					fmt.Fprintf(os.Stderr, "    %s\n",
						warnStyle.Render("warning: could not abort cleanly: "+abortErr.Error()))
				}
			}
			if asJSON {
				jsonOutput.Results = append(jsonOutput.Results, syncJSONResult{
					Path: wt.Path, Branch: branch, Result: "failed", Error: failure,
				})
			}
			failed++
		} else {
			if asJSON {
				jsonOutput.Results = append(jsonOutput.Results, syncJSONResult{
					Path: wt.Path, Branch: branch, Result: "synced",
				})
			} else {
				fmt.Println(okStyle.Render("ok"))
			}
			synced++
		}
	}
	jsonOutput.Summary = syncJSONSummary{Synced: synced, Skipped: skipped, Failed: failed}
	if asJSON {
		if err := writeJSON(jsonOutput); err != nil {
			return err
		}
	}

	if !dryRun && !asJSON {
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

func commandFailure(err error, output []byte) string {
	detail := strings.TrimSpace(string(output))
	if detail == "" {
		return err.Error()
	}
	return detail
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
