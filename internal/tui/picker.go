package tui

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// Item represents a worktree entry in the picker.
type Item struct {
	ID          string // worktree path (unique key)
	Label       string // branch name or short path
	Desc        string // reason line: "merged PR · 21d · no unpushed"
	Selected    bool
	HasUnpushed bool
	Protected   bool
}

// Result is returned after the picker exits.
type Result struct {
	Confirmed bool
	Items     []Item
}

// PickerOptions controls which entries the picker allows users to select.
type PickerOptions struct {
	AllowProtected bool
}

// Model is the bubbletea model for the interactive picker.
type Model struct {
	title          string
	items          []Item
	cursor         int
	height         int
	reviewing      bool
	allowProtected bool
	notice         string
	done           bool
	quit           bool
}

// NewPicker creates a new picker model.
func NewPicker(title string, items []Item) Model {
	return NewPickerWithOptions(title, items, PickerOptions{})
}

// NewPickerWithOptions creates a picker with explicit selection behavior.
func NewPickerWithOptions(title string, items []Item, options PickerOptions) Model {
	return Model{
		title:          title,
		items:          items,
		height:         24,
		allowProtected: options.AllowProtected,
	}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
	case tea.KeyMsg:
		if m.reviewing {
			switch msg.String() {
			case "ctrl+c", "q":
				m.quit = true
				return m, tea.Quit
			case "y", "enter":
				m.done = true
				return m, tea.Quit
			case "n", "esc", "backspace":
				m.reviewing = false
				m.notice = ""
			}
			return m, nil
		}

		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.quit = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
			m.notice = ""
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
			m.notice = ""
		case "home", "g":
			m.cursor = 0
			m.notice = ""
		case "end", "G":
			if len(m.items) > 0 {
				m.cursor = len(m.items) - 1
			}
			m.notice = ""
		case "pgup":
			m.cursor = max(0, m.cursor-m.visibleItemCount())
			m.notice = ""
		case "pgdown":
			if len(m.items) > 0 {
				m.cursor = min(len(m.items)-1, m.cursor+m.visibleItemCount())
			}
			m.notice = ""
		case " ", "x":
			if len(m.items) == 0 {
				break
			}
			if m.items[m.cursor].Protected && !m.allowProtected {
				m.notice = "Protected rows are view-only. Rerun with --force to select one explicitly."
				break
			}
			m.items[m.cursor].Selected = !m.items[m.cursor].Selected
			m.notice = ""
		case "a":
			skipped := 0
			for i := range m.items {
				if m.items[i].Protected {
					skipped++
					continue
				}
				m.items[i].Selected = true
			}
			if skipped > 0 {
				m.notice = fmt.Sprintf("Selected all safe rows; skipped %d protected row(s).", skipped)
			} else {
				m.notice = "Selected all rows."
			}
		case "n":
			for i := range m.items {
				m.items[i].Selected = false
			}
			m.notice = "Selection cleared."
		case "enter":
			if m.selectedCount() == 0 {
				m.notice = "Nothing selected. Move with ↑/↓ and press Space to select a row."
				break
			}
			m.reviewing = true
			m.notice = ""
		}
	}
	return m, nil
}

var (
	titleStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	helpStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	helpKeyStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true)
	focusBarStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	focusedStyle     = lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("15")).Bold(true)
	focusedDescStyle = lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("252"))
	selectedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	checkStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	normalStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	descStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	warnStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	dimStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

func (m Model) View() string {
	if m.quit || m.done {
		return ""
	}
	if m.reviewing {
		return m.reviewView()
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render(m.title) + "\n\n")
	hk := func(k string) string { return helpKeyStyle.Render(k) }
	b.WriteString(helpStyle.Render(fmt.Sprintf(
		"%s move   %s select/unselect   %s review selection",
		hk("↑/↓ or j/k"), hk("Space"), hk("Enter"),
	)) + "\n")
	b.WriteString(helpStyle.Render(fmt.Sprintf(
		"%s all safe   %s clear   %s page   %s quit",
		hk("a"), hk("n"), hk("PgUp/PgDn"), hk("q"),
	)) + "\n\n")

	if len(m.items) == 0 {
		b.WriteString(dimStyle.Render("  No candidates found.\n"))
		return b.String()
	}

	start, end := m.visibleRange()
	if start > 0 {
		fmt.Fprintf(&b, "  %s\n", dimStyle.Render(fmt.Sprintf("↑ %d more", start)))
	}
	for i := start; i < end; i++ {
		item := m.items[i]
		focused := i == m.cursor

		var box string
		if item.Selected {
			box = checkStyle.Render("[✓]")
		} else {
			box = normalStyle.Render("[ ]")
		}

		warn := ""
		if item.Protected {
			warn = "  " + warnStyle.Render("⚠ protected")
		} else if item.Selected && item.HasUnpushed {
			warn = "  " + warnStyle.Render("⚠ unpushed commits")
		}

		bar := "  "
		rowStyle := normalStyle
		if focused {
			bar = focusBarStyle.Render("› ")
			rowStyle = focusedStyle
		} else if item.Selected {
			rowStyle = selectedStyle
		}

		label := rowStyle.Render(item.Label)
		fmt.Fprintf(&b, "%s%s %s%s\n", bar, box, label, warn)

		if item.Desc != "" {
			ds := descStyle
			if focused {
				ds = focusedDescStyle
			}
			fmt.Fprintf(&b, "       %s\n", ds.Render(item.Desc))
		}
	}
	if end < len(m.items) {
		fmt.Fprintf(&b, "  %s\n", dimStyle.Render(fmt.Sprintf("↓ %d more", len(m.items)-end)))
	}

	b.WriteString("\n")
	count := m.selectedCount()
	if count == 0 {
		b.WriteString(helpStyle.Render(fmt.Sprintf(
			"  Row %d/%d · nothing selected — press Space to select the highlighted row",
			m.cursor+1, len(m.items),
		)))
	} else {
		b.WriteString(selectedStyle.Render(fmt.Sprintf(
			"  Row %d/%d · %d selected — press Enter to review before deletion",
			m.cursor+1, len(m.items), count,
		)))
	}
	b.WriteString("\n")
	if m.notice != "" {
		b.WriteString(warnStyle.Render("  "+m.notice) + "\n")
	}

	return b.String()
}

func (m Model) reviewView() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Review selected worktrees") + "\n\n")

	selected := m.selectedItems()
	const maxReviewRows = 10
	for i, item := range selected {
		if i == maxReviewRows {
			fmt.Fprintf(&b, "  %s\n", dimStyle.Render(fmt.Sprintf("… and %d more", len(selected)-maxReviewRows)))
			break
		}
		warning := ""
		if item.Protected {
			warning = " " + warnStyle.Render("⚠ protected")
		}
		fmt.Fprintf(&b, "  %s %s%s\n", checkStyle.Render("✓"), item.Label, warning)
	}

	b.WriteString("\n")
	b.WriteString(warnStyle.Render(fmt.Sprintf("  Delete %d selected worktree(s)? This cannot be undone.", len(selected))) + "\n\n")
	hk := func(k string) string { return helpKeyStyle.Render(k) }
	b.WriteString(helpStyle.Render(fmt.Sprintf(
		"  %s delete   %s back to selection   %s cancel",
		hk("y/Enter"), hk("n/Esc"), hk("q"),
	)) + "\n")
	return b.String()
}

func (m Model) selectedItems() []Item {
	selected := make([]Item, 0, len(m.items))
	for _, item := range m.items {
		if item.Selected {
			selected = append(selected, item)
		}
	}
	return selected
}

func (m Model) selectedCount() int {
	count := 0
	for _, item := range m.items {
		if item.Selected {
			count++
		}
	}
	return count
}

func (m Model) visibleItemCount() int {
	// Reserve space for the title, two help rows, scroll indicators, summary,
	// and notices. Each worktree normally consumes a label and description row.
	count := (m.height - 10) / 2
	if count < 1 {
		return 1
	}
	return count
}

func (m Model) visibleRange() (int, int) {
	count := min(m.visibleItemCount(), len(m.items))
	start := m.cursor - count/2
	if start < 0 {
		start = 0
	}
	if start+count > len(m.items) {
		start = len(m.items) - count
	}
	return start, start + count
}

// Run launches the TUI and returns the result.
func Run(title string, items []Item) (Result, error) {
	return RunWithOptions(title, items, PickerOptions{})
}

// RunWithOptions launches the TUI with explicit protected-row behavior.
func RunWithOptions(title string, items []Item, options PickerOptions) (Result, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return Result{}, fmt.Errorf("interactive picker requires a terminal; run this command directly in a terminal or use bonsai prune --dry-run")
	}
	m := NewPickerWithOptions(title, items, options)
	p := tea.NewProgram(m, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return Result{}, err
	}
	fm := final.(Model)
	return Result{
		Confirmed: fm.done && !fm.quit,
		Items:     fm.items,
	}, nil
}
