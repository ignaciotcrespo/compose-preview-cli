package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ignaciotcrespo/compose-preview-cli/internal/types"
	"github.com/ignaciotcrespo/compose-preview-cli/internal/ui/panel"
)

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	// Search bar (always visible)
	searchLabel := statusBarStyle.Render(" / ")
	var searchBar string
	if m.searchActive {
		searchBar = searchLabel + inputLabelStyle.Render("Filter: ") + m.searchInput.View()
	} else if m.state.Filter != "" {
		searchBar = searchLabel + inputLabelStyle.Render("Filter: ") + detailValueStyle.Render(m.state.Filter) +
			statusBarStyle.Render("  (/ to edit, esc to clear)")
	} else {
		searchBar = searchLabel + statusBarStyle.Render("Press / to filter previews")
	}

	// Header
	title := lipgloss.NewStyle().Bold(true).Foreground(selectedAccent).Render(" Compose Preview Browser")
	projectInfo := statusBarStyle.Render(" — " + m.scanResult.ProjectName)
	deviceInfo := ""
	if len(m.devices) == 0 {
		if m.emulatorBooting {
			deviceInfo = statusBarStyle.Render(" · ") + lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Render("booting emulator...")
		} else {
			deviceInfo = statusBarStyle.Render(" · ") + errorStyle.Render("no device") + statusBarStyle.Render(" (d to select)")
		}
	} else if m.deviceStatus != "" {
		deviceInfo = statusBarStyle.Render(" · ") + detailValueStyle.Render(m.deviceStatus) + statusBarStyle.Render(" (d to change)")
	}
	webInfo := ""
	if m.webServer != nil {
		url := m.webServer.URL()
		link := fmt.Sprintf("\x1b]8;;%s\x1b\\%s\x1b]8;;\x1b\\", url, url)
		webInfo = statusBarStyle.Render(" · ") + lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Render(link) +
			statusBarStyle.Render(" (w to stop)")
	}
	header := title + projectInfo + deviceInfo + webInfo

	// Status bar line (always 1 line, shows latest status or error)
	statusLine := ""
	if m.errorMsg != "" {
		statusLine = errorStyle.Render(" ✗ " + m.errorMsg)
	} else if m.building {
		statusLine = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Render(" ⏳ " + m.statusMsg)
	} else if m.statusMsg != "" {
		statusLine = lipgloss.NewStyle().Foreground(lipgloss.Color("35")).Render(" ● " + m.statusMsg)
	} else if m.needsBuild {
		statusLine = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Render(" ⚠ " + m.buildWarning + " — press 'b' to rebuild")
	}

	// Calculate panel dimensions
	// Layout: header(1) + search(1) + panels(contentH+2) + details(detailH+2) + status(1) + help(1) = height
	detailH := 3
	contentH := m.height - detailH - 8

	var leftW, midW, rightW int
	if m.electronMode {
		// 2-panel layout: Electron shows the screenshot
		leftW = m.width / 4
		midW = m.width - leftW - 4
		rightW = 0
	} else {
		// 3-panel layout: modules(1/5) | previews(2/5) | screenshot(2/5)
		leftW = m.width / 5
		midW = (m.width - leftW - 6) / 2
		rightW = m.width - leftW - midW - 6
	}

	// Modules panel
	modFocused := m.state.Focus == types.PanelModules
	modPC := m.renderModulesContent(contentH)
	modBox := panel.Box(1, "Modules", modPC.content, leftW, contentH, modFocused,
		panel.BoxOpts{Scroll: modPC.scroll, Accent: panelAccent(modFocused)})

	// Previews panel
	prevFocused := m.state.Focus == types.PanelPreviews
	filtered := m.filteredPreviews()
	prevTitle := fmt.Sprintf("Previews (%d)", len(filtered))
	prevPC := m.renderPreviewsContent(contentH)
	prevBox := panel.Box(2, prevTitle, prevPC.content, midW, contentH, prevFocused,
		panel.BoxOpts{Scroll: prevPC.scroll, Accent: panelAccent(prevFocused)})

	var topPanels string
	if m.electronMode {
		topPanels = lipgloss.JoinHorizontal(lipgloss.Top, modBox, prevBox)
	} else {
		// Screenshot panel
		previewContent, previewAge := m.currentPreviewScreenshot(rightW, contentH)
		screenshotTitle := "Preview"
		if m.capturing {
			screenshotTitle = "Preview (capturing...)"
		} else if previewAge != "" {
			screenshotTitle = fmt.Sprintf("Preview (%s)", previewAge)
		}
		if previewContent == "" {
			previewContent = statusBarStyle.Render("  No screenshot\n  s to capture\n  enter auto-captures")
		}
		screenshotBox := panel.Box(3, screenshotTitle, previewContent, rightW, contentH, false)
		topPanels = lipgloss.JoinHorizontal(lipgloss.Top, modBox, prevBox, screenshotBox)
	}

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
	parts = append(parts, searchBar)
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
	layout := lipgloss.JoinVertical(lipgloss.Left, parts...)

	// Overlay device picker modal if active
	if m.showDevicePicker {
		modal := m.renderDevicePickerModal()
		layout = m.overlayModal(layout, modal)
	}

	return layout
}

// renderDevicePickerModal renders the device/emulator selection modal.
func (m Model) renderDevicePickerModal() string {
	var lines []string
	hasDevices := false
	hasAVDs := false

	for i, item := range m.devicePickerItems {
		if item.isDevice && !hasDevices {
			hasDevices = true
			lines = append(lines, inputLabelStyle.Render("Connected devices:"))
		}
		if !item.isDevice && !hasAVDs {
			hasAVDs = true
			if hasDevices {
				lines = append(lines, "") // separator
			}
			lines = append(lines, inputLabelStyle.Render("Emulators (will launch):"))
		}

		cursor := "  "
		style := normalItemStyle
		if i == m.devicePickerSel {
			cursor = "▸ "
			style = selectedItemStyle
		}
		lines = append(lines, cursor+style.Render(item.label))
	}

	if len(m.devicePickerItems) == 0 {
		lines = append(lines, statusBarStyle.Render("  No devices or emulators found"))
	}

	content := ""
	for _, l := range lines {
		content += l + "\n"
	}

	// Calculate modal width based on longest item
	modalW := 40
	for _, item := range m.devicePickerItems {
		if len(item.label)+4 > modalW {
			modalW = len(item.label) + 4
		}
	}
	if modalW > m.width-4 {
		modalW = m.width - 4
	}

	modalH := len(lines)
	help := helpStyle.Render(" ↑↓ navigate · enter select · esc cancel")

	return panel.Box(0, "Select Device / Emulator", content+help, modalW, modalH+1, true,
		panel.BoxOpts{Accent: selectedAccent})
}

// overlayModal places a modal string centered on top of the base layout.
func (m Model) overlayModal(base, modal string) string {
	baseLines := strings.Split(base, "\n")
	modalLines := strings.Split(modal, "\n")

	// Center vertically
	startY := (len(baseLines) - len(modalLines)) / 2
	if startY < 0 {
		startY = 0
	}

	// Center horizontally
	modalWidth := 0
	for _, l := range modalLines {
		w := lipgloss.Width(l)
		if w > modalWidth {
			modalWidth = w
		}
	}
	startX := (m.width - modalWidth) / 2
	if startX < 0 {
		startX = 0
	}

	// Overlay modal lines onto base
	for i, ml := range modalLines {
		row := startY + i
		if row >= len(baseLines) {
			break
		}
		// Replace the portion of the base line with the modal line
		baseLine := baseLines[row]
		baseW := lipgloss.Width(baseLine)

		if startX >= baseW {
			baseLines[row] = baseLine + strings.Repeat(" ", startX-baseW) + ml
		} else {
			// Pad modal line to overlay cleanly
			baseLines[row] = strings.Repeat(" ", startX) + ml
		}
	}

	return strings.Join(baseLines, "\n")
}
