// Package types defines shared enums used across layers.
package types

import "github.com/ignaciotcrespo/tui-framework"

// PanelID identifies a panel in the layout.
type PanelID = tui.PanelID

const (
	PanelModules  PanelID = iota
	PanelPreviews
	PanelDetails
)

// PanelState represents the display state of a toggleable panel.
type PanelState = tui.PanelState

const (
	PanelNormal    = tui.PanelNormal
	PanelMaximized = tui.PanelMaximized
	PanelHidden    = tui.PanelHidden
)

// PromptMode identifies the current input prompt type.
type PromptMode = tui.PromptMode

const (
	PromptNone         PromptMode = iota
	PromptFilter
	PromptBuildVariant // quick-select: which install task to run
	PromptConfirm
)

// ConfirmAction identifies what dangerous action is pending confirmation.
type ConfirmAction = tui.ConfirmAction

const (
	ConfirmNone  ConfirmAction = iota
	ConfirmBuild
)

// RefreshFlag tells the coordinator what data to reload.
type RefreshFlag = tui.RefreshFlag

const (
	RefreshNone = tui.RefreshNone
	RefreshAll  = tui.RefreshAll
)
