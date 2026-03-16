package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"github.com/ignaciotcrespo/compose-preview-cli/internal/types"
	"github.com/ignaciotcrespo/compose-preview-cli/internal/ui/panel"
)

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	// Header
	title := lipgloss.NewStyle().Bold(true).Foreground(selectedAccent).Render(" Compose Preview Browser")
	projectInfo := statusBarStyle.Render(" — " + m.scanResult.ProjectName)
	deviceInfo := ""
	if m.deviceStatus != "" {
		deviceInfo = statusBarStyle.Render(" · ") + detailValueStyle.Render(m.deviceStatus)
	}
	if m.appId != "" {
		deviceInfo += statusBarStyle.Render(" · ") + statusBarStyle.Render(m.appId)
	}
	header := title + projectInfo + deviceInfo

	// Status bar line (always 1 line, shows latest status or error)
	statusLine := ""
	if m.errorMsg != "" {
		statusLine = errorStyle.Render(" ✗ " + m.errorMsg)
	} else if m.statusMsg != "" {
		statusLine = lipgloss.NewStyle().Foreground(lipgloss.Color("35")).Render(" ● " + m.statusMsg)
	}

	// Calculate panel dimensions
	detailH := 3 // FQN, File, Params
	statusLines := 1
	contentH := m.height - 4 - detailH - statusLines // 1 header + 2 detail borders + 1 help + 1 status

	leftW := m.width / 4
	rightW := m.width - leftW - 4 // -4 for borders

	// Modules panel
	modFocused := m.state.Focus == types.PanelModules
	modPC := m.renderModulesContent(contentH)
	modBox := panel.Box(1, "Modules", modPC.content, leftW, contentH, modFocused,
		panel.BoxOpts{Scroll: modPC.scroll, Accent: panelAccent(modFocused)})

	// Previews panel
	prevFocused := m.state.Focus == types.PanelPreviews
	prevTitle := "Previews"
	if m.state.Filter != "" {
		prevTitle = fmt.Sprintf("Previews [%s]", m.state.Filter)
	}
	prevPC := m.renderPreviewsContent(contentH)
	prevBox := panel.Box(2, prevTitle, prevPC.content, rightW, contentH, prevFocused,
		panel.BoxOpts{Scroll: prevPC.scroll, Accent: panelAccent(prevFocused)})

	topPanels := lipgloss.JoinHorizontal(lipgloss.Top, modBox, prevBox)

	// Details panel
	detailContent := m.renderDetailsContent(m.width - 4)
	detailBox := panel.Box(0, "Details", detailContent, m.width-2, detailH, false)

	// Record panel regions for mouse
	leftColW := leftW + 2
	m.panelRegions[types.PanelModules] = panel.Region{X: 0, Y: 0, W: leftColW, H: contentH + 2}
	m.panelRegions[types.PanelPreviews] = panel.Region{X: leftColW, Y: 0, W: rightW + 2, H: contentH + 2}
	m.panelRegions[types.PanelDetails] = panel.Region{X: 0, Y: contentH + 2, W: m.width, H: detailH + 2}

	// Build layout
	var parts []string
	parts = append(parts, header)
	parts = append(parts, topPanels)
	parts = append(parts, detailBox)
	if statusLine != "" {
		parts = append(parts, statusLine)
	} else {
		parts = append(parts, "") // keep layout stable
	}
	if m.prompt.Active() {
		parts = append(parts, m.prompt.Render())
	}
	parts = append(parts, m.renderHelp())
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
