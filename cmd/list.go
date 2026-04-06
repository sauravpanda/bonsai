package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/sauravpanda/bonsai/internal/config"
	"github.com/sauravpanda/bonsai/internal/git"
	"github.com/sauravpanda/bonsai/internal/github"
	"github.com/sauravpanda/bonsai/internal/tui"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all git worktrees",
	Long: `Display a table of all git worktrees with path, branch, age,
last commit message, ahead/behind the base branch, and PR status.`,
	RunE: runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
	listCmd.Flags().Bool("no-pr", false, "filter: show only worktrees with no PR")
	listCmd.Flags().Bool("offline", false, "skip GitHub PR status lookup (faster)")
}

// Fixed column widths.
const (
	colAge = 6
	colPR  = 10
)

// cols holds computed dynamic column widths for a given terminal size.
type cols struct {
	path, branch, commit, diff int
}

// computeCols divides the terminal width among columns.
// diffWidth is the actual max diff string width; numWidth is the # column width.
func computeCols(diffWidth, numWidth int) cols {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w < 80 {
		w = 120
	}
	// overhead: 2 indent + numWidth + # gap(2) + 5×2 gaps + age + diff + pr
	overhead := 2 + numWidth + 2 + 5*2 + colAge + diffWidth + colPR
	budget := w - overhead
	if budget < 60 {
		budget = 60
	}
	// Proportions: PATH 35%, BRANCH 25%, COMMIT 40%
	path := max(budget*35/100, 18)
	branch := max(budget*25/100, 14)
	commit := max(budget-path-branch, 18)
	return cols{path, branch, commit, diffWidth}
}

var (
	headerStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	mainStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	prMerged     = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true) // green
	prOpen       = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))            // yellow
	prClosed     = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))             // red
	prNone       = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))             // dim
	unpushedWarn = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
)

func runList(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	worktrees, err := git.List()
	if err != nil {
		return err
	}

	filterNoPR, _ := cmd.Flags().GetBool("no-pr")
	offline, _ := cmd.Flags().GetBool("offline")
	ghOK := !offline && github.IsAvailable()

	if !ghOK && !offline {
		fmt.Fprintln(os.Stderr, "  note: gh CLI not authenticated — PR status unavailable")
	}

	root, _ := git.MainRoot()

	spin := tui.Start("fetching worktree info…")
	for i, wt := range worktrees {
		if len(worktrees) > 1 {
			spin.UpdateMsg(fmt.Sprintf("enriching worktree %d/%d…", i+1, len(worktrees)))
		}
		git.Enrich(wt, cfg.DefaultBase, cfg.DefaultRemote)
		if ghOK && !wt.IsMain && !wt.IsDetached && wt.Branch != "" {
			spin.UpdateMsg(fmt.Sprintf("fetching PR status for %s…", wt.Branch))
			pr, err := github.GetPR(wt.Branch)
			if err == nil {
				wt.PRStatus = strings.ToLower(pr.State)
				wt.PRURL = pr.URL
			} else {
				wt.PRStatus = "none"
			}
		} else if wt.IsMain {
			wt.PRStatus = "—"
		} else if !ghOK {
			wt.PRStatus = "unknown"
		} else {
			wt.PRStatus = "none"
		}
	}
	spin.Stop()

	// Apply --no-pr filter: keep main worktree + added worktrees with no PR.
	if filterNoPR {
		var filtered []*git.Worktree
		for _, wt := range worktrees {
			if wt.IsMain || wt.PRStatus == "none" || wt.PRStatus == "unknown" {
				filtered = append(filtered, wt)
			}
		}
		worktrees = filtered
	}

	if len(worktrees) == 0 || (len(worktrees) == 1 && worktrees[0].IsMain) {
		fmt.Println(mainStyle.Render("  no worktrees match the filter"))
		return nil
	}

	printTable(worktrees, root)
	return nil
}

// tableRow holds pre-computed, display-ready cell values for one worktree.
type tableRow struct {
	num                                    string // "" for main, "1","2",… for added
	path, branch, age, commit, diff, prStr string
	isMain                                 bool
}

// AddedWorktrees returns only the non-main worktrees in list order.
// The caller's index+1 is the display number shown by bonsai list.
func AddedWorktrees(worktrees []*git.Worktree) []*git.Worktree {
	var out []*git.Worktree
	for _, wt := range worktrees {
		if !wt.IsMain {
			out = append(out, wt)
		}
	}
	return out
}

func printTable(worktrees []*git.Worktree, root string) {
	added := AddedWorktrees(worktrees)
	numWidth := len(fmt.Sprint(len(added))) // digits needed for largest index

	// Pass 1: compute all cell values and measure the actual max diff width.
	rows := make([]tableRow, len(worktrees))
	maxDiffW := 3 // minimum to fit "+/-" header
	hasUnpushedAny := false
	addedIdx := 0

	for i, wt := range worktrees {
		diff := fmt.Sprintf("+%d/-%d", wt.AheadBase, wt.BehindBase)
		if len(diff) > maxDiffW {
			maxDiffW = len(diff)
		}
		branch := wt.Branch
		if wt.HasUnpushed && !wt.IsMain {
			branch = unpushedWarn.Render("*") + wt.Branch
			hasUnpushedAny = true
		}
		num := ""
		if !wt.IsMain {
			addedIdx++
			num = fmt.Sprint(addedIdx)
		}
		rows[i] = tableRow{
			num:    num,
			path:   git.ShortenPath(wt.Path, root),
			branch: branch,
			age:    git.FormatAge(wt.Age),
			commit: wt.LastCommit,
			diff:   diff,
			prStr:  formatPR(wt),
			isMain: wt.IsMain,
		}
	}

	// Pass 2: compute column widths now that we know maxDiffW and numWidth.
	c := computeCols(maxDiffW, numWidth)
	totalWidth := 2 + numWidth + 2 + 5*2 + colAge + c.diff + colPR + c.path + c.branch + c.commit
	sep := strings.Repeat("─", totalWidth)

	header := fmt.Sprintf("%-*s  %-*s  %-*s  %-*s  %-*s  %-*s  %s",
		numWidth, "#",
		c.path, "PATH",
		c.branch, "BRANCH",
		colAge, "AGE",
		c.commit, "LAST COMMIT",
		c.diff, "+/-",
		"PR",
	)
	fmt.Println("  " + headerStyle.Render(header))
	fmt.Println(mainStyle.Render("  " + sep))

	numStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	for _, r := range rows {
		numCell := fitCol(r.num, numWidth)
		if !r.isMain && r.num != "" {
			numCell = numStyle.Render(r.num) + strings.Repeat(" ", numWidth-len(r.num))
		}
		row := "  " +
			numCell + "  " +
			fitCol(r.path, c.path) + "  " +
			fitCol(r.branch, c.branch) + "  " +
			fitCol(r.age, colAge) + "  " +
			fitCol(r.commit, c.commit) + "  " +
			fitCol(r.diff, c.diff) + "  " +
			r.prStr
		if r.isMain {
			fmt.Println(mainStyle.Render(row))
		} else {
			fmt.Println(row)
		}
	}

	fmt.Println(mainStyle.Render("  " + sep))

	summary := fmt.Sprintf("  %d worktree(s)  ·  %d added", len(worktrees), len(added))
	if hasUnpushedAny {
		summary += "  ·  * unpushed commits"
	}
	fmt.Println(mainStyle.Render(summary))
}

// fitCol truncates s to n visible chars (if needed) then pads to exactly n visible chars.
func fitCol(s string, n int) string {
	return padRight(truncate(s, n), n)
}

// padRight pads s with spaces to reach visual width n, accounting for ANSI codes.
func padRight(s string, n int) string {
	visible := len(stripANSI(s))
	if visible >= n {
		return s
	}
	return s + strings.Repeat(" ", n-visible)
}

// hyperlink wraps text in an OSC 8 terminal hyperlink when url is non-empty.
// Terminals that don't support OSC 8 display the plain text unchanged.
func hyperlink(url, text string) string {
	if url == "" {
		return text
	}
	return "\x1b]8;;" + url + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}

func formatPR(wt *git.Worktree) string {
	switch wt.PRStatus {
	case "merged":
		return prMerged.Render("merged")
	case "open":
		label := prOpen.Render("open")
		if wt.PRURL != "" {
			label = hyperlink(wt.PRURL, label)
		}
		return label
	case "closed":
		return prClosed.Render("closed")
	case "—":
		return mainStyle.Render("—")
	case "unknown":
		return mainStyle.Render("?")
	default:
		return prNone.Render("none")
	}
}

// truncate shortens s so its visible (non-ANSI) length is at most n,
// appending "…" when truncation occurs. It correctly skips ANSI escape
// sequences so the cut always falls on a visible character boundary.
func truncate(s string, n int) string {
	if len(stripANSI(s)) <= n {
		return s
	}
	// Walk s counting visible chars; cut just before the nth.
	visible := 0
	inEsc := false
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if s[i] == 'm' {
				inEsc = false
			}
			continue
		}
		if visible == n-1 {
			result := s[:i] + "…"
			// Only emit a reset when the string actually contains ANSI codes,
			// so we don't inject literal "[0m" into plain strings.
			if len(s) != len(stripANSI(s)) {
				result += "\x1b[0m"
			}
			return result
		}
		visible++
	}
	return s
}

// stripANSI removes ANSI escape codes for length calculation.
func stripANSI(s string) string {
	var out strings.Builder
	inEsc := false
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if s[i] == 'm' {
				inEsc = false
			}
			continue
		}
		out.WriteByte(s[i])
	}
	return out.String()
}
