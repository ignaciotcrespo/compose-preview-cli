package panel

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/ignaciotcrespo/compose-preview-cli/internal/types"
	"github.com/ignaciotcrespo/gitshelf/pkg/tui"
)

// Region stores the screen coordinates of a panel for mouse hit-testing.
type Region = tui.Region

// Styles used by panel rendering. Set these from the parent package.
var (
	ActiveBorderStyle   lipgloss.Style
	InactiveBorderStyle lipgloss.Style
	TitleStyle          lipgloss.Style
	StatusBarStyle      lipgloss.Style
)

func syncStyles() {
	tui.ActiveBorderStyle = ActiveBorderStyle
	tui.InactiveBorderStyle = InactiveBorderStyle
	tui.TitleStyle = TitleStyle
	tui.StatusBarStyle = StatusBarStyle
}

// ScrollInfo provides scroll position data for rendering a scrollbar.
type ScrollInfo = tui.ScrollInfo

// BoxOpts holds optional rendering parameters for Box.
type BoxOpts = tui.BoxOpts

// Box renders a bordered panel with a numbered title.
func Box(num int, title, content string, width, height int, active bool, opts ...BoxOpts) string {
	syncStyles()
	return tui.Box(num, title, content, width, height, active, opts...)
}

// CyclePanelState cycles a toggleable panel through states.
func CyclePanelState(current types.PanelState, focused bool) (types.PanelState, bool) {
	return tui.CyclePanelState(current, focused)
}
