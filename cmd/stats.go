package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/sauravpanda/bonsai/internal/config"
	"github.com/sauravpanda/bonsai/internal/git"
	"github.com/sauravpanda/bonsai/internal/github"
	"github.com/sauravpanda/bonsai/internal/tui"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Aggregate metrics across all worktrees",
	Long: `Show a health snapshot of all worktrees: counts by state, disk usage, and age distribution.

Pass --json to emit one aggregate object without progress output.`,
	RunE: runStats,
}

func init() {
	rootCmd.AddCommand(statsCmd)
	statsCmd.Flags().Bool("offline", false, "skip GitHub PR status lookup")
	statsCmd.Flags().Bool("json", false, "emit aggregate metrics as JSON to stdout (no progress output)")
}

type statsJSONAgeDistribution struct {
	UnderOneDay       int `json:"under_1_day"`
	OneToSevenDays    int `json:"one_to_seven_days"`
	SevenToThirtyDays int `json:"seven_to_thirty_days"`
	OverThirtyDays    int `json:"over_30_days"`
}

// statsJSON is the stable machine-readable shape emitted by `bonsai stats --json`.
type statsJSON struct {
	TotalWorktrees     int                      `json:"total_worktrees"`
	StaleWorktrees     int                      `json:"stale_worktrees"`
	OpenPRs            int                      `json:"open_prs"`
	UnpushedWorktrees  int                      `json:"unpushed_worktrees"`
	DiskBytes          int64                    `json:"disk_bytes"`
	StaleThresholdDays int                      `json:"stale_threshold_days"`
	AgeDistribution    statsJSONAgeDistribution `json:"age_distribution"`
}

func dirSize(path string) int64 {
	var total int64
	filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total
}

func formatBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GiB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func runStats(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	offline, _ := cmd.Flags().GetBool("offline")
	asJSON, _ := cmd.Flags().GetBool("json")
	ghOK := !offline && github.IsAvailable()

	worktrees, err := git.List()
	if err != nil {
		return err
	}

	added := AddedWorktrees(worktrees)
	if len(added) == 0 {
		if asJSON {
			return writeJSON(statsJSON{StaleThresholdDays: cfg.StaleThresholdDays})
		}
		fmt.Println("  no added worktrees")
		return nil
	}

	var spin *tui.Spinner
	if !asJSON {
		spin = tui.Start(fmt.Sprintf("collecting stats for %d worktree(s)…", len(added)))
	}
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
					wt.PRNumber = pr.Number
					wt.PRHeadOID = pr.HeadRefOID
				} else if errors.Is(err, github.ErrNoPR) {
					wt.PRStatus = "none"
				} else {
					wt.PRStatus = "unknown"
				}
			}
			return nil
		})
	}
	g.Wait() //nolint:errcheck
	if spin != nil {
		spin.Stop()
	}

	staleDur := staleDuration(cfg.StaleThresholdDays)
	metrics := buildStatsJSON(added, staleDur, cfg.StaleThresholdDays)
	if asJSON {
		return writeJSON(metrics)
	}

	sep := dimStyle.Render("  " + strings.Repeat("─", 40))
	fmt.Println(sep)
	fmt.Printf("  %s  %s\n", boldStyle.Render("Total worktrees:"), fmt.Sprint(metrics.TotalWorktrees))
	fmt.Printf("  %s  %s\n", boldStyle.Render("Stale (≥ threshold): "), staleStyle(metrics.StaleWorktrees, errStyle))
	fmt.Printf("  %s  %s\n", boldStyle.Render("Open PRs:            "), staleStyle(metrics.OpenPRs, okStyle))
	fmt.Printf("  %s  %s\n", boldStyle.Render("Unpushed commits:    "), staleStyle(metrics.UnpushedWorktrees, warnStyle))
	fmt.Printf("  %s  %s\n", boldStyle.Render("Total disk usage:    "), formatBytes(metrics.DiskBytes))
	fmt.Println(sep)
	fmt.Println("  " + boldStyle.Render("Age distribution:"))
	fmt.Printf("    < 1 day   : %d\n", metrics.AgeDistribution.UnderOneDay)
	fmt.Printf("    1-7 days  : %d\n", metrics.AgeDistribution.OneToSevenDays)
	fmt.Printf("    7-30 days : %d\n", metrics.AgeDistribution.SevenToThirtyDays)
	fmt.Printf("    > 30 days : %d\n", metrics.AgeDistribution.OverThirtyDays)
	fmt.Println(sep)
	return nil
}

func buildStatsJSON(worktrees []*git.Worktree, staleDur float64, staleThresholdDays int) statsJSON {
	metrics := statsJSON{
		TotalWorktrees:     len(worktrees),
		StaleThresholdDays: staleThresholdDays,
	}
	for _, wt := range worktrees {
		if float64(wt.Age) >= staleDur {
			metrics.StaleWorktrees++
		}
		if wt.PRStatus == "open" {
			metrics.OpenPRs++
		}
		if wt.HasUnpushed {
			metrics.UnpushedWorktrees++
		}
		metrics.DiskBytes += dirSize(wt.Path)

		switch {
		case wt.Age < 24*time.Hour:
			metrics.AgeDistribution.UnderOneDay++
		case wt.Age < 7*24*time.Hour:
			metrics.AgeDistribution.OneToSevenDays++
		case wt.Age < 30*24*time.Hour:
			metrics.AgeDistribution.SevenToThirtyDays++
		default:
			metrics.AgeDistribution.OverThirtyDays++
		}
	}
	return metrics
}

func staleStyle(n int, nonZeroStyle lipgloss.Style) string {
	if n == 0 {
		return dimStyle.Render("0")
	}
	return nonZeroStyle.Render(fmt.Sprint(n))
}
