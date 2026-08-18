package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	runewidth "github.com/mattn/go-runewidth"
	"github.com/sauravpanda/bonsai/internal/config"
	"github.com/sauravpanda/bonsai/internal/git"
	"github.com/sauravpanda/bonsai/internal/github"
	"github.com/sauravpanda/bonsai/internal/tui"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	"golang.org/x/term"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all git worktrees",
	Long: `Display a table of all git worktrees with path, branch, age,
last commit message, ahead/behind the base branch, and PR status.

Pass --json to emit structured output for scripting; in that mode progress
messages are suppressed and JSON is written to stdout.`,
	RunE: runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
	listCmd.Flags().Bool("no-pr", false, "filter: show only worktrees with no PR")
	listCmd.Flags().Bool("offline", false, "skip GitHub PR status lookup (faster)")
	listCmd.Flags().Bool("json", false, "emit worktree data as JSON to stdout (no progress output)")
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
	asJSON, _ := cmd.Flags().GetBool("json")
	ghOK := !offline && github.IsAvailable()

	if !ghOK && !offline && !asJSON {
		fmt.Fprintln(os.Stderr, "  note: gh CLI not authenticated — PR status unavailable")
	}

	root, _ := git.MainRoot()

	var spin *tui.Spinner
	if !asJSON {
		spin = tui.Start(fmt.Sprintf("enriching %d worktree(s) in parallel…", len(worktrees)))
	}
	var g errgroup.Group
	for _, wt := range worktrees {
		wt := wt
		g.Go(func() error {
			git.Enrich(wt, cfg.DefaultBase, cfg.DefaultRemote)
			if ghOK && !wt.IsMain && !wt.IsDetached && wt.Branch != "" {
				pr, err := github.GetPR(wt.Branch)
				if err == nil {
					wt.PRStatus = strings.ToLower(pr.State)
					wt.PRURL = pr.URL
					wt.PRNumber = pr.Number
					wt.PRHeadOID = pr.HeadRefOID
					reconcileMergedPRStatus(wt)
				} else if errors.Is(err, github.ErrNoPR) {
					wt.PRStatus = "none"
				} else {
					wt.PRStatus = "unknown"
				}
			} else if wt.IsMain {
				wt.PRStatus = "—"
			} else if !ghOK {
				wt.PRStatus = "unknown"
			} else {
				wt.PRStatus = "none"
			}
			return nil
		})
	}
	g.Wait() //nolint:errcheck — goroutines always return nil
	if spin != nil {
		spin.Stop()
	}
	activeDirs, activeKnown := activeWorkingDirectories()
	for _, wt := range worktrees {
		wt.InUseKnown = activeKnown
		wt.IsInUse = pathContainsAny(wt.Path, activeDirs)
	}

	// Apply --no-pr filter: keep main worktree + added worktrees with no PR.
	if filterNoPR {
		var filtered []*git.Worktree
		for _, wt := range worktrees {
			if matchesNoPRFilter(wt) {
				filtered = append(filtered, wt)
			}
		}
		worktrees = filtered
	}

	if asJSON {
		return emitJSON(worktrees, root)
	}

	if len(worktrees) == 0 || (len(worktrees) == 1 && worktrees[0].IsMain) {
		fmt.Println(mainStyle.Render("  no worktrees match the filter"))
		return nil
	}

	staleDur := staleDuration(cfg.StaleThresholdDays)
	printTable(worktrees, root, staleDur)
	return nil
}

func matchesNoPRFilter(wt *git.Worktree) bool {
	return wt.IsMain || wt.PRStatus == "none"
}

func reconcileMergedPRStatus(wt *git.Worktree) {
	// Squash and rebase merges do not make the original branch commits
	// ancestors of the base branch. If GitHub confirms that this exact HEAD was
	// the merged PR head, it is not unpushed work and should not get a warning.
	if wt.PRStatus == "merged" && wt.PRHeadOID != "" && wt.HEAD == wt.PRHeadOID {
		wt.HasUnpushed = false
		wt.UnpushedCommits = 0
		wt.UnpushedKnown = true
	}
}

// jsonWorktree is the JSON shape emitted by `bonsai list --json`.
type jsonWorktree struct {
	Number             int    `json:"number,omitempty"`
	Path               string `json:"path"`
	ShortPath          string `json:"short_path"`
	Branch             string `json:"branch"`
	HEAD               string `json:"head,omitempty"`
	IsMain             bool   `json:"is_main"`
	IsDetached         bool   `json:"is_detached,omitempty"`
	IsLocked           bool   `json:"is_locked,omitempty"`
	LockReason         string `json:"lock_reason,omitempty"`
	IsInUse            bool   `json:"is_in_use,omitempty"`
	InUseKnown         bool   `json:"in_use_known"`
	AgeSeconds         int64  `json:"age_seconds"`
	AgeHuman           string `json:"age_human"`
	CreatedAt          string `json:"created_at,omitempty"`
	ActivityAt         string `json:"activity_at,omitempty"`
	LastCommit         string `json:"last_commit,omitempty"`
	AheadBase          int    `json:"ahead_base"`
	BehindBase         int    `json:"behind_base"`
	PRStatus           string `json:"pr_status,omitempty"`
	PRNumber           int    `json:"pr_number,omitempty"`
	PRURL              string `json:"pr_url,omitempty"`
	HasUnpushed        bool   `json:"has_unpushed"`
	UnpushedCommits    int    `json:"unpushed_commits"`
	UnpushedKnown      bool   `json:"unpushed_known"`
	RemoteBranchExists bool   `json:"remote_branch_exists"`
	MergedIntoBase     bool   `json:"merged_into_base"`
	DirtyFiles         int    `json:"dirty_files"`
	StagedFiles        int    `json:"staged_files"`
	UntrackedFiles     int    `json:"untracked_files"`
	StatusKnown        bool   `json:"status_known"`
}

func emitJSON(worktrees []*git.Worktree, root string) error {
	out := make([]jsonWorktree, 0, len(worktrees))
	addedIdx := 0
	for _, wt := range worktrees {
		j := jsonWorktree{
			Path:               wt.Path,
			ShortPath:          git.ShortenPath(wt.Path, root),
			Branch:             wt.Branch,
			HEAD:               wt.HEAD,
			IsMain:             wt.IsMain,
			IsDetached:         wt.IsDetached,
			IsLocked:           wt.IsLocked,
			LockReason:         wt.LockReason,
			IsInUse:            wt.IsInUse,
			InUseKnown:         wt.InUseKnown,
			AgeSeconds:         int64(wt.Age / time.Second),
			AgeHuman:           git.FormatAge(wt.Age),
			LastCommit:         wt.LastCommit,
			AheadBase:          wt.AheadBase,
			BehindBase:         wt.BehindBase,
			PRStatus:           wt.PRStatus,
			PRNumber:           wt.PRNumber,
			PRURL:              wt.PRURL,
			HasUnpushed:        wt.HasUnpushed,
			UnpushedCommits:    wt.UnpushedCommits,
			UnpushedKnown:      wt.UnpushedKnown,
			RemoteBranchExists: wt.RemoteBranchExists,
			MergedIntoBase:     wt.MergedIntoBase,
			DirtyFiles:         wt.DirtyFiles,
			StagedFiles:        wt.StagedFiles,
			UntrackedFiles:     wt.UntrackedFiles,
			StatusKnown:        wt.StatusKnown,
		}
		if !wt.CreatedAt.IsZero() {
			j.CreatedAt = wt.CreatedAt.UTC().Format(time.RFC3339)
		}
		if !wt.ActivityAt.IsZero() {
			j.ActivityAt = wt.ActivityAt.UTC().Format(time.RFC3339)
		}
		// The em-dash placeholder used by the table renderer is noise in JSON.
		if j.PRStatus == "—" {
			j.PRStatus = ""
		}
		if !wt.IsMain {
			addedIdx++
			j.Number = addedIdx
		}
		out = append(out, j)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
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

func printTable(worktrees []*git.Worktree, root string, staleDur float64) {
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
			age:    colorAge(wt.Age, staleDur, wt.IsMain),
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

	numStyle := infoStyle.Bold(true)
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

// padRight pads s with spaces to reach visual width n, accounting for ANSI codes
// and multi-byte Unicode characters.
func padRight(s string, n int) string {
	visible := runewidth.StringWidth(stripANSI(s))
	if visible >= n {
		return s
	}
	return s + strings.Repeat(" ", n-visible)
}

// colorAge renders the age string with a color that signals staleness:
//   - main worktree or zero age: dim
//   - < 3 days: green
//   - < stale threshold: yellow
//   - >= stale threshold: red
func colorAge(age time.Duration, staleDur float64, isMain bool) string {
	s := git.FormatAge(age)
	if isMain || age == 0 {
		return mainStyle.Render(s)
	}
	threeDays := staleDuration(3)
	switch {
	case float64(age) < threeDays:
		return ageFresh.Render(s)
	case float64(age) < staleDur:
		return ageWarn.Render(s)
	default:
		return ageStale.Render(s)
	}
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

// truncate shortens s so its visible (non-ANSI) display width is at most n,
// appending "…" when truncation occurs. Uses runewidth for correct Unicode
// double-width character handling.
func truncate(s string, n int) string {
	plain := stripANSI(s)
	if runewidth.StringWidth(plain) <= n {
		return s
	}
	// Walk the original string rune-by-rune, tracking display width.
	// We need to rebuild the prefix including any ANSI escape sequences.
	var result strings.Builder
	inEsc := false
	visWidth := 0
	hasANSI := len(s) != len(plain)
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '\x1b' {
			inEsc = true
			result.WriteRune(r)
			continue
		}
		if inEsc {
			result.WriteRune(r)
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		rw := runewidth.RuneWidth(r)
		if visWidth+rw > n-1 {
			result.WriteRune('…')
			if hasANSI {
				result.WriteString("\x1b[0m")
			}
			return result.String()
		}
		result.WriteRune(r)
		visWidth += rw
	}
	return s
}

// truncateLeft shortens s so its display width is at most n, dropping the
// head and prepending "…". Use for paths, where the tail is the part that
// distinguishes entries. Expects plain (non-ANSI) input.
func truncateLeft(s string, n int) string {
	if runewidth.StringWidth(s) <= n {
		return s
	}
	runes := []rune(s)
	visWidth := 0
	for i := len(runes) - 1; i >= 0; i-- {
		rw := runewidth.RuneWidth(runes[i])
		if visWidth+rw > n-1 {
			return "…" + string(runes[i+1:])
		}
		visWidth += rw
	}
	return s
}

// stripANSI removes ANSI escape codes, returning the plain visible string.
func stripANSI(s string) string {
	var out strings.Builder
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}
