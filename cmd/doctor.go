package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/sauravpanda/bonsai/internal/git"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Health check for broken and orphaned worktrees",
	Long: `Run a series of checks on all git worktrees and report issues:

  - Broken worktrees (missing .git file or stale gitdir pointer)
  - Detached HEAD worktrees
  - Deleted branch (branch no longer exists locally)
  - Worktrees pointing to missing directories

For each issue, a suggested fix command is printed.`,
	RunE: runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

type doctorIssue struct {
	severity string // "error" or "warn"
	label    string
	detail   string
	fix      string
}

var (
	issueError = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	issueWarn  = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	issueFix   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	issueOK    = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
)

func runDoctor(cmd *cobra.Command, args []string) error {
	worktrees, err := git.List()
	if err != nil {
		return err
	}

	var issues []doctorIssue
	checked := 0

	for _, wt := range worktrees {
		if wt.IsMain {
			continue
		}
		checked++

		// 1. Check that the worktree directory exists.
		if _, err := os.Stat(wt.Path); os.IsNotExist(err) {
			issues = append(issues, doctorIssue{
				severity: "error",
				label:    wt.Path,
				detail:   "directory does not exist",
				fix:      fmt.Sprintf("git worktree prune"),
			})
			continue
		}

		// 2. Check for broken .git file (should be a file referencing gitdir).
		gitFile := filepath.Join(wt.Path, ".git")
		info, err := os.Stat(gitFile)
		if err != nil || info.IsDir() {
			issues = append(issues, doctorIssue{
				severity: "error",
				label:    wt.Path,
				detail:   "missing or malformed .git file",
				fix:      fmt.Sprintf("git worktree prune"),
			})
			continue
		}

		// Check that the gitdir pointer resolves.
		content, _ := os.ReadFile(gitFile)
		gitdirLine := strings.TrimSpace(string(content))
		if strings.HasPrefix(gitdirLine, "gitdir: ") {
			gitdir := strings.TrimPrefix(gitdirLine, "gitdir: ")
			if !filepath.IsAbs(gitdir) {
				gitdir = filepath.Join(wt.Path, gitdir)
			}
			if _, err := os.Stat(gitdir); os.IsNotExist(err) {
				issues = append(issues, doctorIssue{
					severity: "error",
					label:    wt.Path,
					detail:   fmt.Sprintf("stale gitdir pointer: %s", gitdir),
					fix:      "git worktree prune",
				})
				continue
			}
		}

		// 3. Detached HEAD.
		if wt.IsDetached {
			issues = append(issues, doctorIssue{
				severity: "warn",
				label:    wt.Branch,
				detail:   fmt.Sprintf("detached HEAD at %s in %s", wt.HEAD[:min8(len(wt.HEAD))], wt.Path),
				fix:      fmt.Sprintf("git -C %q checkout -b <branch-name>", wt.Path),
			})
			continue
		}

		// 4. Branch no longer exists locally.
		if wt.Branch != "" && wt.Branch != "(detached)" {
			if !branchExists(wt.Branch) {
				issues = append(issues, doctorIssue{
					severity: "error",
					label:    wt.Branch,
					detail:   fmt.Sprintf("branch %q no longer exists (path: %s)", wt.Branch, wt.Path),
					fix:      fmt.Sprintf("git worktree remove %q", wt.Path),
				})
			}
		}
	}

	if len(issues) == 0 {
		fmt.Printf("%s  All %d worktree(s) are healthy.\n", issueOK.Render("✓"), checked)
		return nil
	}

	fmt.Printf("Found %d issue(s) across %d worktree(s):\n\n", len(issues), checked)
	for _, iss := range issues {
		var sev string
		if iss.severity == "error" {
			sev = issueError.Render("✗ error")
		} else {
			sev = issueWarn.Render("⚠ warn")
		}
		fmt.Printf("  %s  %s\n", sev, iss.label)
		fmt.Printf("         %s\n", iss.detail)
		fmt.Printf("         %s %s\n\n", issueFix.Render("fix:"), iss.fix)
	}
	return fmt.Errorf("%d issue(s) found", len(issues))
}

func branchExists(branch string) bool {
	err := exec.Command("git", "rev-parse", "--verify", "refs/heads/"+branch).Run()
	return err == nil
}

func min8(n int) int {
	if n < 8 {
		return n
	}
	return 8
}
