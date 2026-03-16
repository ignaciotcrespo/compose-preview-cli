package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ignaciotcrespo/compose-preview-cli/internal/scanner"
	"github.com/ignaciotcrespo/compose-preview-cli/internal/types"
	"github.com/ignaciotcrespo/compose-preview-cli/internal/ui/panel"
	"github.com/ignaciotcrespo/tui-framework"
)

type panelContent struct {
	content string
	scroll  panel.ScrollInfo
}

func visibleRange(selected, count, maxLines, linesPerItem int) (int, int) {
	return tui.VisibleRange(selected, count, maxLines, linesPerItem)
}

// filteredCountForModule returns how many previews in a module match the current filter.
func (m Model) filteredCountForModule(mod scanner.Module) int {
	if m.state.Filter == "" {
		return len(mod.Previews)
	}
	filter := strings.ToLower(m.state.Filter)
	count := 0
	for _, p := range mod.Previews {
		name := strings.ToLower(p.FunctionName)
		pname := strings.ToLower(p.PreviewName)
		if strings.Contains(name, filter) || strings.Contains(pname, filter) {
			count++
		}
	}
	return count
}

func (m Model) renderModulesContent(maxLines int) panelContent {
	total := len(m.modules)
	if total == 0 {
		return panelContent{content: statusBarStyle.Render("  No modules found")}
	}

	focused := m.state.Focus == types.PanelModules
	baseStyle := normalItemStyle
	if focused {
		baseStyle = focusedItemStyle
	}

	var b strings.Builder
	start, end := visibleRange(m.state.ModuleSel, total, maxLines, 1)
	for i := start; i < end; i++ {
		mod := m.modules[i]
		cursor := "  "
		style := baseStyle
		if i == m.state.ModuleSel {
			cursor = "▸ "
			if focused {
				style = selectedItemStyle
			} else {
				style = dimSelectedItemStyle
			}
		}
		filteredCount := m.filteredCountForModule(mod)
		count := fmt.Sprintf(" (%d)", filteredCount)
		countStyle := statusBarStyle
		if filteredCount > 0 {
			countStyle = moduleCountStyle
		}
		b.WriteString(cursor + style.Render(mod.Name) + countStyle.Render(count) + "\n")
	}
	return panelContent{
		content: b.String(),
		scroll:  panel.ScrollInfo{TotalLines: total, VisibleLines: maxLines, ScrollPos: start},
	}
}

func (m Model) renderPreviewsContent(maxLines int) panelContent {
	previews := m.filteredPreviews()
	total := len(previews)
	if total == 0 {
		msg := "  No previews found"
		if m.state.Filter != "" {
			msg = fmt.Sprintf("  No previews matching '%s'", m.state.Filter)
		}
		return panelContent{content: statusBarStyle.Render(msg)}
	}

	focused := m.state.Focus == types.PanelPreviews
	baseStyle := normalItemStyle
	if focused {
		baseStyle = focusedItemStyle
	}

	var b strings.Builder
	start, end := visibleRange(m.state.PreviewSel, total, maxLines, 1)
	for i := start; i < end; i++ {
		p := previews[i]
		cursor := "  "
		style := baseStyle
		if i == m.state.PreviewSel {
			cursor = "▸ "
			if focused {
				style = selectedItemStyle
			} else {
				style = dimSelectedItemStyle
			}
		}
		name := p.FunctionName
		if p.PreviewName != "" {
			name += " (" + p.PreviewName + ")"
		}
		b.WriteString(cursor + style.Render(name) + "\n")
	}
	return panelContent{
		content: b.String(),
		scroll:  panel.ScrollInfo{TotalLines: total, VisibleLines: maxLines, ScrollPos: start},
	}
}

func (m Model) renderDetailsContent(width int) string {
	previews := m.filteredPreviews()
	if m.state.PreviewSel >= len(previews) || len(previews) == 0 {
		return statusBarStyle.Render("  Select a preview")
	}

	p := previews[m.state.PreviewSel]
	var lines []string

	fqnLine := detailLabelStyle.Render("FQN: ") + detailValueStyle.Render(p.FQN)
	lines = append(lines, fqnLine)

	relPath := p.FilePath
	if m.projectRoot != "" {
		if rel, err := filepath.Rel(m.projectRoot, p.FilePath); err == nil {
			relPath = rel
		}
	}
	fileLine := detailLabelStyle.Render("File: ") + detailValueStyle.Render(fmt.Sprintf("%s:%d", relPath, p.LineNumber))
	lines = append(lines, fileLine)

	if len(p.Params) > 0 {
		var params []string
		for k, v := range p.Params {
			if k == "name" {
				continue
			}
			params = append(params, k+"="+v)
		}
		if len(params) > 0 {
			paramLine := detailLabelStyle.Render("Params: ") + detailValueStyle.Render(strings.Join(params, ", "))
			lines = append(lines, paramLine)
		}
	}

	return strings.Join(lines, "\n")
}

func (m Model) renderHelp() string {
	if m.prompt.Active() {
		return helpStyle.Render(" enter confirm · esc cancel")
	}
	if m.searchActive {
		return helpStyle.Render(" type to filter · tab panels · esc clear · enter confirm")
	}

	parts := []string{"enter run", "b build", "/ filter", "R refresh", "q quit"}
	return helpStyle.Render(" " + strings.Join(parts, " · "))
}

func (m Model) filteredPreviews() []scanner.PreviewFunc {
	if m.state.ModuleSel >= len(m.modules) {
		return nil
	}
	previews := m.modules[m.state.ModuleSel].Previews
	if m.state.Filter == "" {
		return previews
	}
	filter := strings.ToLower(m.state.Filter)
	var filtered []scanner.PreviewFunc
	for _, p := range previews {
		name := strings.ToLower(p.FunctionName)
		pname := strings.ToLower(p.PreviewName)
		if strings.Contains(name, filter) || strings.Contains(pname, filter) {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

func panelAccent(focused bool) lipgloss.Color {
	if focused {
		return selectedAccent
	}
	return contextAccent
}
