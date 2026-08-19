package cmd

import (
	"fmt"
	"strings"
	"time"

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
  - current branch and last commit

Pass --json to emit one object per worktree without progress output.`,
	RunE: runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
	statusCmd.Flags().Bool("json", false, "emit dashboard data as JSON to stdout (no progress output)")
}

type wtStatus struct {
	wt        *git.Worktree
	dirty     int
	staged    int
	untracked int
}

// statusJSONWorktree is the stable machine-readable shape emitted by
// `bonsai status --json`.
type statusJSONWorktree struct {
	Path           string `json:"path"`
	Branch         string `json:"branch"`
	HEAD           string `json:"head,omitempty"`
	IsMain         bool   `json:"is_main"`
	IsDetached     bool   `json:"is_detached,omitempty"`
	AgeSeconds     int64  `json:"age_seconds"`
	AgeHuman       string `json:"age_human"`
	LastCommit     string `json:"last_commit,omitempty"`
	AheadBase      int    `json:"ahead_base"`
	BehindBase     int    `json:"behind_base"`
	StagedFiles    int    `json:"staged_files"`
	DirtyFiles     int    `json:"dirty_files"`
	UntrackedFiles int    `json:"untracked_files"`
	StatusKnown    bool   `json:"status_known"`
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

	asJSON, _ := cmd.Flags().GetBool("json")
	var spin *tui.Spinner
	if !asJSON {
		spin = tui.Start(fmt.Sprintf("collecting status for %d worktree(s)…", len(worktrees)))
	}
	var g errgroup.Group
	for i, wt := range worktrees {
		i, wt := i, wt
		g.Go(func() error {
			git.Enrich(wt, cfg.DefaultBase, cfg.DefaultRemote)
			statuses[i].dirty = wt.DirtyFiles
			statuses[i].staged = wt.StagedFiles
			statuses[i].untracked = wt.UntrackedFiles
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}
	if spin != nil {
		spin.Stop()
	}

	if asJSON {
		return writeJSON(buildStatusJSON(statuses))
	}

	staleDur := staleDuration(cfg.StaleThresholdDays)

	fmt.Println(headerStyle.Render(fmt.Sprintf("  %-28s  %-6s  %-14s  %-8s  %s", "BRANCH", "AGE", "STAGED/DIRTY/UNTRACKED", "+/-", "LAST COMMIT")))
	fmt.Println(dimStyle.Render("  " + strings.Repeat("─", 90)))

	for _, s := range statuses {
		wt := s.wt

		branch := wt.Branch
		if wt.IsMain {
			branch = dimStyle.Render(branch)
		}

		age := colorAge(wt.Age, staleDur, wt.IsMain)
		diff := fmt.Sprintf("+%d/-%d", wt.AheadBase, wt.BehindBase)

		var stateStr string
		if wt.IsMain {
			stateStr = dimStyle.Render("—")
		} else {
			stagedPart := fmt.Sprintf("%d staged", s.staged)
			dirtyPart := fmt.Sprintf("%d dirty", s.dirty)
			untrackedPart := fmt.Sprintf("%d untracked", s.untracked)
			if s.staged > 0 {
				stagedPart = infoStyle.Render(stagedPart)
			} else {
				stagedPart = dimStyle.Render(stagedPart)
			}
			if s.dirty > 0 {
				dirtyPart = warnStyle.Render(dirtyPart)
			} else {
				dirtyPart = okStyle.Render(dirtyPart)
			}
			if s.untracked > 0 {
				untrackedPart = warnStyle.Render(untrackedPart)
			} else {
				untrackedPart = dimStyle.Render(untrackedPart)
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
			fmt.Println(dimStyle.Render(row))
		} else {
			fmt.Println(row)
		}
	}

	fmt.Println(dimStyle.Render("  " + strings.Repeat("─", 90)))
	return nil
}

func buildStatusJSON(statuses []wtStatus) []statusJSONWorktree {
	out := make([]statusJSONWorktree, 0, len(statuses))
	for _, status := range statuses {
		wt := status.wt
		out = append(out, statusJSONWorktree{
			Path:           wt.Path,
			Branch:         wt.Branch,
			HEAD:           wt.HEAD,
			IsMain:         wt.IsMain,
			IsDetached:     wt.IsDetached,
			AgeSeconds:     int64(wt.Age / time.Second),
			AgeHuman:       git.FormatAge(wt.Age),
			LastCommit:     wt.LastCommit,
			AheadBase:      wt.AheadBase,
			BehindBase:     wt.BehindBase,
			StagedFiles:    status.staged,
			DirtyFiles:     status.dirty,
			UntrackedFiles: status.untracked,
			StatusKnown:    wt.StatusKnown,
		})
	}
	return out
}
