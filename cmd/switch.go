package cmd

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sauravpanda/bonsai/internal/git"
	"github.com/spf13/cobra"
)

var switchCmd = &cobra.Command{
	Use:   "switch",
	Short: "Fuzzy picker to cd into a worktree",
	Long: `Launch an interactive picker to navigate to a worktree.
The selected worktree path is printed to stdout as a cd command.

Shell integration (add to ~/.bashrc or ~/.zshrc):
  bonsai_switch() { eval "$(bonsai switch)"; }
  # Optional: bind to a key, e.g. Ctrl+W in zsh:
  # bindkey -s '^W' 'bonsai_switch\n'

Usage:
  bonsai switch          # interactive picker
  eval $(bonsai switch)  # cd into the selected worktree`,
	RunE: runSwitch,
}

func init() {
	rootCmd.AddCommand(switchCmd)
}

// switchModel is a single-select bubbletea model for bonsai switch.
type switchModel struct {
	items    []switchItem
	cursor   int
	selected string
	quit     bool
}

type switchItem struct {
	path   string
	branch string
	label  string
}

var (
	swCursor = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	swNormal = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	swDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	swTitle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	swHelp   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

func (m switchModel) Init() tea.Cmd { return nil }

func (m switchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		case "enter":
			if len(m.items) > 0 {
				m.selected = m.items[m.cursor].path
			}
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m switchModel) View() string {
	var b strings.Builder
	b.WriteString(swTitle.Render("bonsai switch — select a worktree") + "\n")
	b.WriteString(swHelp.Render("↑/↓  navigate  ·  enter  select  ·  q  quit") + "\n\n")

	for i, item := range m.items {
		if i == m.cursor {
			b.WriteString(swCursor.Render("▶ ") + swNormal.Render(item.label) + "\n")
		} else {
			b.WriteString("  " + swDim.Render(item.label) + "\n")
		}
	}
	return b.String()
}

func runSwitch(cmd *cobra.Command, args []string) error {
	worktrees, err := git.List()
	if err != nil {
		return err
	}

	var items []switchItem
	root, _ := git.MainRoot()
	for _, wt := range worktrees {
		short := git.ShortenPath(wt.Path, root)
		label := fmt.Sprintf("%-28s  %s", truncate(wt.Branch, 28), swDim.Render(short))
		items = append(items, switchItem{
			path:   wt.Path,
			branch: wt.Branch,
			label:  label,
		})
	}

	if len(items) == 0 {
		fmt.Fprintln(os.Stderr, "  no worktrees found")
		return nil
	}

	m := switchModel{items: items}
	// Redirect TUI to stderr so the cd command goes cleanly to stdout.
	p := tea.NewProgram(m, tea.WithOutput(os.Stderr))
	final, err := p.Run()
	if err != nil {
		return err
	}

	fm, ok := final.(switchModel)
	if !ok || fm.quit || fm.selected == "" {
		return nil
	}

	// Print the cd command to stdout so `eval $(bonsai switch)` works.
	fmt.Printf("cd %q\n", fm.selected)
	return nil
}
