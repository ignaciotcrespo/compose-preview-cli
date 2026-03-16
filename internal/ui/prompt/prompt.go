package prompt

import (
	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ignaciotcrespo/compose-preview-cli/internal/types"
	"github.com/ignaciotcrespo/tui-framework"
)

// Styles used by prompt rendering. Set these from the parent package.
var (
	InputLabelStyle lipgloss.Style
	ErrorStyle      lipgloss.Style
	HelpStyle       lipgloss.Style
)

func syncStyles() {
	tui.InputLabelStyle = InputLabelStyle
	tui.ErrorStyle = ErrorStyle
	tui.HelpStyle = HelpStyle
}

// Result represents the outcome of a completed prompt.
type Result = tui.Result

// composeLabeler implements tui.PromptLabeler for compose-preview prompts.
type composeLabeler struct{}

func (c composeLabeler) PromptLabel(mode types.PromptMode) string {
	switch mode {
	case types.PromptFilter:
		return "Filter previews"
	case types.PromptBuildVariant:
		return "Build variant"
	}
	return ""
}

func (c composeLabeler) ConfirmMessage(action types.ConfirmAction, target string) string {
	switch action {
	case types.ConfirmBuild:
		return "Build " + target
	}
	return ""
}

// Prompt holds the current input/confirmation state.
type Prompt struct {
	inner tui.Prompt
}

// Active returns true if a prompt is currently showing.
func (p *Prompt) Active() bool {
	return p.inner.Active()
}

// Start begins a new input prompt.
func (p *Prompt) Start(mode types.PromptMode, defaultValue string) tea.Cmd {
	p.ensureInit()
	syncStyles()
	return p.inner.Start(mode, defaultValue)
}

// StartWithOptions begins a prompt with quick-select options.
func (p *Prompt) StartWithOptions(mode types.PromptMode, defaultValue string, names []string) tea.Cmd {
	p.ensureInit()
	syncStyles()
	return p.inner.StartWithOptions(mode, defaultValue, names)
}

// StartConfirm begins a confirmation prompt.
func (p *Prompt) StartConfirm(action types.ConfirmAction, target string) {
	p.ensureInit()
	syncStyles()
	p.inner.StartConfirm(action, target)
}

// Cancel dismisses the current prompt.
func (p *Prompt) Cancel() {
	p.inner.Cancel()
}

// HandleKey processes a key event for the active prompt.
func (p *Prompt) HandleKey(msg tea.KeyMsg) (*Result, bool, tea.Cmd) {
	return p.inner.HandleKey(msg)
}

// Update processes non-key messages for the textinput.
func (p *Prompt) Update(msg tea.Msg) tea.Cmd {
	return p.inner.Update(msg)
}

// Render returns the prompt bar string.
func (p *Prompt) Render() string {
	syncStyles()
	return p.inner.Render()
}

func (p *Prompt) ensureInit() {
	if p.inner.Labeler == nil {
		p.inner = tui.NewPrompt(composeLabeler{}, types.PromptConfirm)
	}
}
