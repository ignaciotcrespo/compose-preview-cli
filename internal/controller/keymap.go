package controller

import (
	"github.com/ignaciotcrespo/compose-preview-cli/internal/types"
	"github.com/ignaciotcrespo/gitshelf/pkg/tui"
)

// HandleKey processes a key event and returns the resulting state and actions.
func HandleKey(key string, state State, ctx KeyContext) KeyResult {
	kr := KeyResult{State: state}

	switch key {
	case "q", "ctrl+c":
		kr.Quit = true
		return kr

	case "tab":
		if len(ctx.TabFlow) > 0 {
			kr.State.Focus = tui.TabPanel(state.Focus, ctx.TabFlow, 1)
			kr.Refresh = types.RefreshAll
		}
		return kr

	case "shift+tab":
		if len(ctx.TabFlow) > 0 {
			kr.State.Focus = tui.TabPanel(state.Focus, ctx.TabFlow, -1)
			kr.Refresh = types.RefreshAll
		}
		return kr

	case "1":
		kr.State.Focus = types.PanelModules
		kr.Refresh = types.RefreshAll
		return kr

	case "2":
		kr.State.Focus = types.PanelPreviews
		kr.Refresh = types.RefreshAll
		return kr

	case "up", "k":
		switch state.Focus {
		case types.PanelModules:
			if kr.State.ModuleSel > 0 {
				kr.State.ModuleSel--
				kr.State.PreviewSel = 0
				kr.Refresh = types.RefreshAll
			}
		case types.PanelPreviews:
			if kr.State.PreviewSel > 0 {
				kr.State.PreviewSel--
				kr.Refresh = types.RefreshAll
			}
		}
		return kr

	case "down", "j":
		switch state.Focus {
		case types.PanelModules:
			if kr.State.ModuleSel < ctx.ModuleCount-1 {
				kr.State.ModuleSel++
				kr.State.PreviewSel = 0
				kr.Refresh = types.RefreshAll
			}
		case types.PanelPreviews:
			if kr.State.PreviewSel < ctx.PreviewCount-1 {
				kr.State.PreviewSel++
				kr.Refresh = types.RefreshAll
			}
		}
		return kr

	case "enter", "r":
		if ctx.PreviewCount > 0 {
			kr.RunOnDevice = true
		} else {
			kr.ErrorMsg = "No previews to run"
		}
		return kr

	case "b":
		kr.RunBuild = true
		return kr

	case "f":
		kr.Prompt = &PromptReq{
			Mode:         types.PromptFilter,
			DefaultValue: state.Filter,
		}
		return kr

	case "esc":
		if state.Filter != "" {
			kr.State.Filter = ""
			kr.State.PreviewSel = 0
			kr.Refresh = types.RefreshAll
		}
		return kr
	}

	return kr
}
