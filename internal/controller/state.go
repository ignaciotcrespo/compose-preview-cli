package controller

import "github.com/ignaciotcrespo/compose-preview-cli/internal/types"

// State holds the navigation and selection state for the TUI.
type State struct {
	Focus      types.PanelID
	ModuleSel  int
	PreviewSel int
	Filter     string // active filter string
}

// NewState creates the initial controller state.
func NewState() State {
	return State{
		Focus: types.PanelModules,
	}
}

// KeyContext provides read-only data for key handling decisions.
type KeyContext struct {
	ModuleCount  int
	PreviewCount int
	TabFlow      []types.PanelID
}

// PromptReq describes a prompt request from the controller.
type PromptReq struct {
	Mode         types.PromptMode
	DefaultValue string
}

// KeyResult is the output of HandleKey.
type KeyResult struct {
	State     State
	Refresh   types.RefreshFlag
	Prompt    *PromptReq
	StatusMsg string
	ErrorMsg  string
	Quit      bool
	RunOnDevice bool // trigger ADB preview launch
	RunBuild    bool // trigger gradle build
}
