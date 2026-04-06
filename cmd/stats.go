package cmd

import (
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
	Long:  `Show a health snapshot of all worktrees: counts by state, disk usage, and age distribution.`,
	RunE:  runStats,
}

func init() {
	rootCmd.AddCommand(statsCmd)
	statsCmd.Flags().Bool("offline", false, "skip GitHub PR status lookup")
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
	ghOK := !offline && github.IsAvailable()

	worktrees, err := git.List()
	if err != nil {
		return err
	}

	added := AddedWorktrees(worktrees)
	if len(added) == 0 {
		fmt.Println("  no added worktrees")
		return nil
	}

	spin := tui.Start(fmt.Sprintf("collecting stats for %d worktree(s)…", len(added)))
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
				} else {
					wt.PRStatus = "none"
				}
			}
			return nil
		})
	}
	g.Wait() //nolint:errcheck
	spin.Stop()

	staleDur := float64(cfg.StaleThresholdDays) * 24 * 3600e9

	var (
		staleCount    int
		openPRCount   int
		unpushedCount int
		diskTotal     int64

		bucket1d  int // < 1 day
		bucket7d  int // 1-7 days
		bucket30d int // 7-30 days
		bucketOld int // > 30 days
	)

	for _, wt := range added {
		if float64(wt.Age) >= staleDur {
			staleCount++
		}
		if wt.PRStatus == "open" {
			openPRCount++
		}
		if wt.HasUnpushed {
			unpushedCount++
		}
		diskTotal += dirSize(wt.Path)

		switch {
		case wt.Age < 24*time.Hour:
			bucket1d++
		case wt.Age < 7*24*time.Hour:
			bucket7d++
		case wt.Age < 30*24*time.Hour:
			bucket30d++
		default:
			bucketOld++
		}
	}

	bold := lipgloss.NewStyle().Bold(true)
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	green := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	yellow := lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	red := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))

	sep := dim.Render("  " + strings.Repeat("─", 40))
	fmt.Println(sep)
	fmt.Printf("  %s  %s\n", bold.Render("Total worktrees:"), fmt.Sprint(len(added)))
	fmt.Printf("  %s  %s\n", bold.Render("Stale (≥ threshold): "), staleStyle(staleCount, red))
	fmt.Printf("  %s  %s\n", bold.Render("Open PRs:            "), staleStyle(openPRCount, green))
	fmt.Printf("  %s  %s\n", bold.Render("Unpushed commits:    "), staleStyle(unpushedCount, yellow))
	fmt.Printf("  %s  %s\n", bold.Render("Total disk usage:    "), formatBytes(diskTotal))
	fmt.Println(sep)
	fmt.Println("  " + bold.Render("Age distribution:"))
	fmt.Printf("    < 1 day   : %d\n", bucket1d)
	fmt.Printf("    1-7 days  : %d\n", bucket7d)
	fmt.Printf("    7-30 days : %d\n", bucket30d)
	fmt.Printf("    > 30 days : %d\n", bucketOld)
	fmt.Println(sep)
	return nil
}

func staleStyle(n int, nonZeroStyle lipgloss.Style) string {
	if n == 0 {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("0")
	}
	return nonZeroStyle.Render(fmt.Sprint(n))
}
