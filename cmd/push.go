package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sauravpanda/bonsai/internal/config"
	"github.com/sauravpanda/bonsai/internal/git"
	"github.com/sauravpanda/bonsai/internal/github"
	"github.com/spf13/cobra"
)

var pushCmd = &cobra.Command{
	Use:   "push [worktree-path-or-branch]",
	Short: "Push a worktree's branch and optionally open a PR",
	Long: `Push the branch of a worktree to the remote.

If a path or branch name is provided, bonsai will find the matching
worktree. If omitted, the current working directory is used.

After pushing, you can optionally open a GitHub PR via gh CLI,
and then remove the worktree.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runPush,
}

func init() {
	rootCmd.AddCommand(pushCmd)
	pushCmd.Flags().Bool("pr", false, "open a PR after pushing")
	pushCmd.Flags().Bool("web", false, "open PR creation in browser (implies --pr)")
	pushCmd.Flags().BoolP("remove", "r", false, "remove worktree after push/PR")
	pushCmd.Flags().Bool("dry-run", false, "show what would happen without doing it")
}

func runPush(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	openPR, _ := cmd.Flags().GetBool("pr")
	web, _ := cmd.Flags().GetBool("web")
	remove, _ := cmd.Flags().GetBool("remove")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	if web {
		openPR = true
	}

	worktrees, err := git.List()
	if err != nil {
		return err
	}

	wt, err := resolveWorktree(worktrees, args)
	if err != nil {
		return err
	}

	if wt.IsMain {
		return fmt.Errorf("cannot push the main worktree with bonsai push; use git push directly")
	}
	if wt.IsDetached {
		return fmt.Errorf("worktree is in detached HEAD state — cannot push")
	}

	fmt.Printf("  worktree : %s\n", wt.Path)
	fmt.Printf("  branch   : %s\n", wt.Branch)
	fmt.Printf("  remote   : %s\n", cfg.DefaultRemote)
	fmt.Println()

	if dryRun {
		fmt.Printf("  [dry-run] git push %s %s\n", cfg.DefaultRemote, wt.Branch)
		if openPR {
			fmt.Printf("  [dry-run] gh pr create --fill\n")
		}
		if remove {
			fmt.Printf("  [dry-run] git worktree remove %s\n", wt.Path)
		}
		return nil
	}

	// Push
	fmt.Printf("  pushing %s → %s/%s ...\n", wt.Branch, cfg.DefaultRemote, wt.Branch)
	pushCmd2 := exec.Command("git", "push", cfg.DefaultRemote, wt.Branch)
	pushCmd2.Dir = wt.Path
	pushCmd2.Stdout = os.Stdout
	pushCmd2.Stderr = os.Stderr
	if err := pushCmd2.Run(); err != nil {
		return fmt.Errorf("git push failed: %w", err)
	}
	fmt.Println()

	// PR
	if openPR {
		ghOK := github.IsAvailable()
		if !ghOK {
			fmt.Fprintln(os.Stderr, "  warning: gh CLI not authenticated — skipping PR creation")
		} else {
			fmt.Println("  opening PR ...")
			prArgs := []string{"pr", "create", "--fill"}

			// Ticket auto-linking: extract ticket IDs from the branch name and
			// prepend them to the PR body so Linear/Jira recognise the reference.
			if ticketRef := extractTicket(wt.Branch, cfg.TicketPattern); ticketRef != "" {
				prArgs = append(prArgs, "--body", ticketRef+"\n\n")
				fmt.Printf("  ticket   : %s\n", ticketRef)
			}

			if web {
				prArgs = append(prArgs, "--web")
			}
			prCmd := exec.Command("gh", prArgs...)
			prCmd.Dir = wt.Path
			prCmd.Stdout = os.Stdout
			prCmd.Stderr = os.Stderr
			if err := prCmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "  warning: gh pr create failed: %v\n", err)
			}
			fmt.Println()
		}
	}

	// Remove
	if remove {
		if wt.HasUnpushed {
			// After push, re-check — but we just pushed, so should be clear.
			// Re-enrich to be safe.
			git.Enrich(wt, cfg.DefaultBase, cfg.DefaultRemote)
		}
		fmt.Printf("  removing worktree %s ...\n", wt.Path)
		if err := git.Remove(wt.Path, false); err != nil {
			return fmt.Errorf("remove worktree: %w", err)
		}
		fmt.Println("  done")
	}

	return nil
}

// extractTicket returns the first ticket ID found in branch using the configured
// regexp pattern. Returns "" if pattern is empty or no match.
func extractTicket(branch, pattern string) string {
	if pattern == "" {
		return ""
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return ""
	}
	m := re.FindStringSubmatch(branch)
	if len(m) < 2 {
		// No capture group match — try full match.
		if re.MatchString(branch) {
			return re.FindString(branch)
		}
		return ""
	}
	return m[1]
}

// resolveWorktree finds the worktree matching the given arg (path or branch),
// or uses the current directory if no arg is given.
func resolveWorktree(worktrees []*git.Worktree, args []string) (*git.Worktree, error) {
	if len(args) == 0 {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		cwd = filepath.Clean(cwd)
		for _, wt := range worktrees {
			if filepath.Clean(wt.Path) == cwd {
				return wt, nil
			}
		}
		return nil, fmt.Errorf("current directory %q is not a known worktree", cwd)
	}

	needle := strings.TrimRight(args[0], "/")
	for _, wt := range worktrees {
		if wt.Branch == needle || wt.Path == needle ||
			filepath.Base(wt.Path) == needle {
			return wt, nil
		}
	}
	return nil, fmt.Errorf("no worktree found matching %q", needle)
}
