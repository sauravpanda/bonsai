package cmd

import "github.com/charmbracelet/lipgloss"

// Centralized palette used by all cmd/ output. Prefer these over redefining
// locally — new semantic roles belong here, not per-file.
var (
	boldStyle = lipgloss.NewStyle().Bold(true)
	dimStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	okStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	warnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	errStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	infoStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))

	okBoldStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	errBoldStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9"))

	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))

	// Semantic aliases — kept so call sites read naturally.
	mainStyle   = dimStyle
	reasonStyle = dimStyle

	ageFresh = okStyle
	ageWarn  = warnStyle
	ageStale = errStyle

	prMerged     = okBoldStyle
	prOpen       = warnStyle
	prClosed     = errStyle
	prNone       = dimStyle
	unpushedWarn = warnStyle
)
