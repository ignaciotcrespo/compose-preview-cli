package ui

import "github.com/charmbracelet/lipgloss"

// Accent colors
var (
	selectedAccent = lipgloss.Color("35")  // green — focused
	contextAccent  = lipgloss.Color("251") // light gray — in-context
	dimColor       = lipgloss.Color("240") // gray — not in context
)

var (
	activeBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("35"))

	inactiveBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(dimColor)

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("255"))

	selectedItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("39")).
				Bold(true)

	dimSelectedItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("255"))

	normalItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	focusedItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	inputLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Bold(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	moduleCountStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("35"))

	detailLabelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("39"))

	detailValueStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))
)
