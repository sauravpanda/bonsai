package cmd

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/sauravpanda/bonsai/internal/config"
	"github.com/sauravpanda/bonsai/internal/git"
	"github.com/sauravpanda/bonsai/internal/tui"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Dashboard view of all worktrees with working tree state",
	Long: `Show all worktrees with a quick summary of their working tree state:
  - dirty (modified/deleted tracked files)
  - staged (files in the index)
  - untracked files
  - ahead/behind the base branch
  - current branch and last commit`,
	RunE: runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

var (
	statusClean   = lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // green
	statusDirty   = lipgloss.NewStyle().Foreground(lipgloss.Color("11")) // yellow
	statusStagedS = lipgloss.NewStyle().Foreground(lipgloss.Color("12")) // blue
)

type wtStatus struct {
	wt        *git.Worktree
	dirty     int
	staged    int
	untracked int
}

func parseStatus(path string) (dirty, staged, untracked int) {
	out, err := exec.Command("git", "-C", path, "status", "--porcelain").Output()
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(out), "\n") {
		if len(line) < 2 {
			continue
		}
		x := line[0] // index status
		y := line[1] // worktree status
		switch {
		case line[:2] == "??":
			untracked++
		case x != ' ' && x != '?':
			staged++
			if y != ' ' && y != '?' {
				dirty++
			}
		default:
			if y != ' ' && y != '?' {
				dirty++
			}
		}
	}
	return
}

func runStatus(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	worktrees, err := git.List()
	if err != nil {
		return err
	}

	statuses := make([]wtStatus, len(worktrees))
	for i, wt := range worktrees {
		statuses[i].wt = wt
	}

	spin := tui.Start(fmt.Sprintf("collecting status for %d worktree(s)…", len(worktrees)))
	var g errgroup.Group
	for i, wt := range worktrees {
		i, wt := i, wt
		g.Go(func() error {
			git.Enrich(wt, cfg.DefaultBase, cfg.DefaultRemote)
			d, s, u := parseStatus(wt.Path)
			statuses[i].dirty = d
			statuses[i].staged = s
			statuses[i].untracked = u
			return nil
		})
	}
	g.Wait() //nolint:errcheck
	spin.Stop()

	root, _ := git.MainRoot()
	staleDur := float64(cfg.StaleThresholdDays) * 24 * 3600e9

	hdr := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	fmt.Println(hdr.Render(fmt.Sprintf("  %-28s  %-6s  %-14s  %-8s  %s", "BRANCH", "AGE", "STAGED/DIRTY/UNTRACKED", "+/-", "LAST COMMIT")))
	fmt.Println(dim.Render("  " + strings.Repeat("─", 90)))

	for _, s := range statuses {
		wt := s.wt
		path := git.ShortenPath(wt.Path, root)
		_ = path

		branch := wt.Branch
		if wt.IsMain {
			branch = dim.Render(branch)
		}

		age := colorAge(wt.Age, staleDur, wt.IsMain)
		diff := fmt.Sprintf("+%d/-%d", wt.AheadBase, wt.BehindBase)

		var stateStr string
		if wt.IsMain {
			stateStr = dim.Render("—")
		} else {
			stagedPart := fmt.Sprintf("%d staged", s.staged)
			dirtyPart := fmt.Sprintf("%d dirty", s.dirty)
			untrackedPart := fmt.Sprintf("%d untracked", s.untracked)
			if s.staged > 0 {
				stagedPart = statusStagedS.Render(stagedPart)
			} else {
				stagedPart = dim.Render(stagedPart)
			}
			if s.dirty > 0 {
				dirtyPart = statusDirty.Render(dirtyPart)
			} else {
				dirtyPart = statusClean.Render(dirtyPart)
			}
			if s.untracked > 0 {
				untrackedPart = statusDirty.Render(untrackedPart)
			} else {
				untrackedPart = dim.Render(untrackedPart)
			}
			stateStr = stagedPart + "  " + dirtyPart + "  " + untrackedPart
		}

		commit := truncate(wt.LastCommit, 40)
		row := fmt.Sprintf("  %-28s  %-6s  %s  %-8s  %s",
			truncate(branch, 28),
			age,
			stateStr,
			diff,
			commit,
		)
		if wt.IsMain {
			fmt.Println(dim.Render(row))
		} else {
			fmt.Println(row)
		}
	}

	fmt.Println(dim.Render("  " + strings.Repeat("─", 90)))
	return nil
}
