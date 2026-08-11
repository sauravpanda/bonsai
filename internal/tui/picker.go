package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Item represents a worktree entry in the picker.
type Item struct {
	ID          string // worktree path (unique key)
	Label       string // branch name or short path
	Desc        string // reason line: "merged PR · 21d · no unpushed"
	Selected    bool
	HasUnpushed bool
}

// Result is returned after the picker exits.
type Result struct {
	Confirmed bool
	Items     []Item
}

// Model is the bubbletea model for the interactive picker.
type Model struct {
	title  string
	items  []Item
	cursor int
	done   bool
	quit   bool
}

// NewPicker creates a new picker model.
func NewPicker(title string, items []Item) Model {
	return Model{title: title, items: items}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.quit = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case " ", "x":
			m.items[m.cursor].Selected = !m.items[m.cursor].Selected
		case "a":
			for i := range m.items {
				m.items[i].Selected = true
			}
		case "n":
			for i := range m.items {
				m.items[i].Selected = false
			}
		case "enter":
			m.done = true
			return m, tea.Quit
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

	var b strings.Builder
	b.WriteString(titleStyle.Render(m.title) + "\n")
	hk := func(k string) string { return helpKeyStyle.Render(k) }
	b.WriteString(helpStyle.Render(fmt.Sprintf(
		"%s select/unselect  ·  %s move  ·  %s all  ·  %s none  ·  %s confirm  ·  %s quit",
		hk("space"), hk("↑/↓"), hk("a"), hk("n"), hk("enter"), hk("q"),
	)) + "\n\n")

	if len(m.items) == 0 {
		b.WriteString(dimStyle.Render("  No candidates found.\n"))
		return b.String()
	}

	for i, item := range m.items {
		focused := i == m.cursor

		var box string
		if item.Selected {
			box = checkStyle.Render("[✓]")
		} else {
			box = normalStyle.Render("[ ]")
		}

		warn := ""
		if item.Selected && item.HasUnpushed {
			warn = "  " + warnStyle.Render("⚠ unpushed commits")
		}

		bar := "  "
		rowStyle := normalStyle
		if focused {
			bar = focusBarStyle.Render("▌ ")
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

	// Summary line
	count := 0
	for _, it := range m.items {
		if it.Selected {
			count++
		}
	}
	b.WriteString("\n")
	if count == 0 {
		b.WriteString(helpStyle.Render("  nothing selected"))
	} else {
		b.WriteString(warnStyle.Render(fmt.Sprintf("  %d worktree(s) will be deleted", count)))
	}
	b.WriteString("\n")

	return b.String()
}

// Run launches the TUI and returns the result.
func Run(title string, items []Item) (Result, error) {
	m := NewPicker(title, items)
	p := tea.NewProgram(m)
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
