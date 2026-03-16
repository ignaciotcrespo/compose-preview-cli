package ui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ignaciotcrespo/compose-preview-cli/internal/adb"
	"github.com/ignaciotcrespo/compose-preview-cli/internal/controller"
	"github.com/ignaciotcrespo/compose-preview-cli/internal/gradle"
	"github.com/ignaciotcrespo/compose-preview-cli/internal/scanner"
	"github.com/ignaciotcrespo/compose-preview-cli/internal/types"
	"github.com/ignaciotcrespo/compose-preview-cli/internal/ui/panel"
	"github.com/ignaciotcrespo/compose-preview-cli/internal/ui/prompt"
)

// buildCompleteMsg is sent when a gradle build finishes.
type buildCompleteMsg struct {
	module string
	err    error
}

// adbLaunchMsg is sent when an ADB launch completes.
type adbLaunchMsg struct {
	preview string
	err     error
}

func init() {
	panel.ActiveBorderStyle = activeBorderStyle
	panel.InactiveBorderStyle = inactiveBorderStyle
	panel.TitleStyle = titleStyle
	panel.StatusBarStyle = statusBarStyle

	prompt.InputLabelStyle = inputLabelStyle
	prompt.ErrorStyle = errorStyle
	prompt.HelpStyle = helpStyle
}

// Model is the main TUI model.
type Model struct {
	state controller.State

	width  int
	height int

	// Data
	scanResult  scanner.ScanResult
	modules     []scanner.Module
	projectRoot string
	appId       string // applicationId from the app module (for ADB launch)

	// Device
	deviceStatus string
	devices      []adb.Device

	// Build status
	statusMsg     string
	errorMsg      string
	building      bool
	needsBuild    bool
	buildWarning  string   // e.g. "sources changed since last build"
	appModulePath string   // path to the app module (for APK staleness check)
	installTasks  []string // cached install tasks (e.g. installDevDebug, installAcceptDebug)
	lastBuildTask string   // remember last selected task

	// Search bar (always visible at top)
	searchInput  textinput.Model
	searchActive bool // true when search bar is focused

	// Prompt
	prompt prompt.Prompt

	// Panel regions for mouse click detection
	panelRegions map[types.PanelID]panel.Region
}

// NewModel creates the initial model from scan results.
func NewModel(result scanner.ScanResult, projectRoot string) Model {
	// Filter out modules with no previews for display, but keep all
	var modulesWithPreviews []scanner.Module
	for _, m := range result.Modules {
		if len(m.Previews) > 0 {
			modulesWithPreviews = append(modulesWithPreviews, m)
		}
	}
	// If no modules have previews, show all modules
	modules := modulesWithPreviews
	if len(modules) == 0 {
		modules = result.Modules
	}

	deviceStatus := ""
	var devices []adb.Device
	if adb.IsADBAvailable() {
		if d, err := adb.DetectDevices(); err == nil && len(d) > 0 {
			devices = d
			if d[0].Model != "" {
				deviceStatus = d[0].Model
			} else {
				deviceStatus = d[0].Serial
			}
		} else {
			deviceStatus = "no device"
		}
	} else {
		deviceStatus = "adb not found"
	}

	// Find applicationId from the app module (the one with applicationId, not just namespace).
	// PreviewActivity lives in the app APK, so we always need the app's package.
	appId, appModulePath := findAppApplicationId(result.Modules, projectRoot)

	// Check if sources are newer than the APK
	needsBuild, buildWarning := gradle.NeedsBuild(appModulePath, projectRoot)

	// Search input
	si := textinput.New()
	si.Prompt = ""
	si.Placeholder = "type to filter previews..."
	si.CharLimit = 100

	return Model{
		state:         controller.NewState(),
		scanResult:    result,
		modules:       modules,
		projectRoot:   projectRoot,
		appId:         appId,
		appModulePath: appModulePath,
		devices:       devices,
		deviceStatus:  deviceStatus,
		needsBuild:    needsBuild,
		buildWarning:  buildWarning,
		searchInput:   si,
		panelRegions:  make(map[types.PanelID]panel.Region),
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case buildCompleteMsg:
		m.building = false
		if msg.err != nil {
			m.errorMsg = fmt.Sprintf("Build failed: %v", msg.err)
			m.statusMsg = ""
		} else {
			m.statusMsg = fmt.Sprintf("Build complete: %s", msg.module)
			m.errorMsg = ""
			m.needsBuild = false
			m.buildWarning = ""
		}
		// Recheck staleness
		m.needsBuild, m.buildWarning = gradle.NeedsBuild(m.appModulePath, m.projectRoot)
		return m, nil

	case adbLaunchMsg:
		if msg.err != nil {
			m.errorMsg = fmt.Sprintf("Launch failed: %v", msg.err)
		} else {
			m.statusMsg = fmt.Sprintf("Launched: %s", msg.preview)
			m.errorMsg = ""
		}
		return m, nil

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		// Prompt handling takes priority (build variant picker, etc.)
		if m.prompt.Active() {
			result, handled, cmd := m.prompt.HandleKey(msg)
			if handled {
				if result != nil {
					if followUpCmd := m.handlePromptResult(result); followUpCmd != nil {
						return m, tea.Batch(cmd, followUpCmd)
					}
				}
				return m, cmd
			}
		}

		// Search bar is active — route keys to textinput
		if m.searchActive {
			switch msg.String() {
			case "tab":
				// Exit search, move to panels
				m.searchActive = false
				m.searchInput.Blur()
				return m, nil
			case "esc":
				// Clear search and exit
				m.searchActive = false
				m.searchInput.SetValue("")
				m.searchInput.Blur()
				m.state.Filter = ""
				m.state.PreviewSel = 0
				return m, nil
			case "enter":
				// Confirm filter and move to panels
				m.searchActive = false
				m.searchInput.Blur()
				return m, nil
			case "ctrl+c":
				return m, tea.Quit
			default:
				// Forward to textinput
				var cmd tea.Cmd
				m.searchInput, cmd = m.searchInput.Update(msg)
				// Live filter: update state from input value
				m.state.Filter = m.searchInput.Value()
				m.state.PreviewSel = 0
				return m, cmd
			}
		}

		// Normal mode — controller handles keys
		key := msg.String()

		// "/" activates search bar
		if key == "/" {
			m.searchActive = true
			return m, m.searchInput.Focus()
		}

		keyCtx := controller.KeyContext{
			ModuleCount:  len(m.modules),
			PreviewCount: len(m.filteredPreviews()),
			TabFlow:      []types.PanelID{types.PanelModules, types.PanelPreviews},
		}
		kr := controller.HandleKey(key, m.state, keyCtx)
		m.state = kr.State

		if kr.Quit {
			return m, tea.Quit
		}
		if kr.StatusMsg != "" {
			m.statusMsg = kr.StatusMsg
		}
		if kr.ErrorMsg != "" {
			m.errorMsg = kr.ErrorMsg
		}
		if kr.Prompt != nil {
			cmd := m.prompt.Start(kr.Prompt.Mode, kr.Prompt.DefaultValue)
			return m, cmd
		}
		if kr.RunOnDevice {
			return m, m.launchPreview()
		}
		if kr.RunBuild {
			return m, m.startBuild()
		}
		return m, nil
	}

	// Forward non-key messages to search input (cursor blink)
	if m.searchActive {
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		return m, cmd
	}

	// Forward non-key messages to prompt
	if m.prompt.Active() {
		cmd := m.prompt.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *Model) handlePromptResult(result *prompt.Result) tea.Cmd {
	switch result.Mode {
	case types.PromptFilter:
		m.state.Filter = result.Value
		m.state.PreviewSel = 0
	case types.PromptBuildVariant:
		return m.runBuildTask(result.Value)
	}
	return nil
}

func (m *Model) launchPreview() tea.Cmd {
	previews := m.filteredPreviews()
	if m.state.PreviewSel >= len(previews) {
		m.errorMsg = "No preview selected"
		m.statusMsg = ""
		return nil
	}
	if len(m.devices) == 0 {
		m.errorMsg = "No device connected"
		m.statusMsg = ""
		return nil
	}
	if m.appId == "" {
		m.errorMsg = "Could not detect applicationId — check build.gradle.kts"
		m.statusMsg = ""
		return nil
	}

	p := previews[m.state.PreviewSel]
	device := m.devices[0]

	m.statusMsg = fmt.Sprintf("Launching %s on %s...", p.FunctionName, m.deviceStatus)
	m.errorMsg = ""

	fqn := p.FQN
	serial := device.Serial
	baseAppId := m.appId

	return func() tea.Msg {
		// Find all installed variants of this app (e.g. com.umob.umob.dev, com.umob.umob.qa)
		packages := adb.FindInstalledPackage(serial, baseAppId)
		if len(packages) == 0 {
			return adbLaunchMsg{
				preview: p.FunctionName,
				err:     fmt.Errorf("app not installed — build and install first (press 'b')"),
			}
		}

		// Try each installed variant until one works
		var lastErr error
		for _, pkg := range packages {
			err := adb.LaunchPreview(serial, pkg, fqn)
			if err == nil {
				return adbLaunchMsg{preview: p.FunctionName + " (" + pkg + ")"}
			}
			lastErr = err
		}
		return adbLaunchMsg{
			preview: p.FunctionName,
			err:     fmt.Errorf("tried %d package(s): %v", len(packages), lastErr),
		}
	}
}

// findAppApplicationId looks for the applicationId across all modules.
// Returns the applicationId and the module path.
func findAppApplicationId(modules []scanner.Module, projectRoot string) (string, string) {
	// Priority 1: Check well-known app module names
	for _, name := range []string{":composeApp", ":app"} {
		for _, mod := range modules {
			if mod.Name == name {
				if id := gradle.FindApplicationId(mod.Path); id != "" {
					return id, mod.Path
				}
			}
		}
	}
	// Priority 2: Check all modules for any applicationId
	for _, mod := range modules {
		if id := gradle.FindApplicationId(mod.Path); id != "" {
			return id, mod.Path
		}
	}
	return "", ""
}

func (m *Model) startBuild() tea.Cmd {
	if m.building {
		m.statusMsg = "Build already in progress..."
		return nil
	}

	gradlew, err := gradle.FindGradlew(m.projectRoot)
	if err != nil {
		m.errorMsg = err.Error()
		return nil
	}

	// Find the app module name for install tasks
	appModuleName := ":composeApp"
	for _, mod := range m.scanResult.Modules {
		if mod.Path == m.appModulePath {
			appModuleName = mod.Name
			break
		}
	}

	// Discover install tasks if not cached
	if len(m.installTasks) == 0 {
		m.statusMsg = "Querying build variants..."
		m.errorMsg = ""
		tasks := gradle.ListInstallTasks(gradlew, appModuleName)
		m.installTasks = tasks
		m.statusMsg = ""
	}

	// If only one task, run it directly
	if len(m.installTasks) == 1 {
		return m.runBuildTask(m.installTasks[0])
	}

	// If we used a task before, run it again directly
	if m.lastBuildTask != "" {
		return m.runBuildTask(m.lastBuildTask)
	}

	// Multiple tasks: show quick-select prompt
	return m.prompt.StartWithOptions(types.PromptBuildVariant, "", m.installTasks)
}

func (m *Model) runBuildTask(task string) tea.Cmd {
	gradlew, err := gradle.FindGradlew(m.projectRoot)
	if err != nil {
		m.errorMsg = err.Error()
		return nil
	}

	m.building = true
	m.lastBuildTask = task
	m.statusMsg = fmt.Sprintf("Running %s...", task)
	m.errorMsg = ""

	return func() tea.Msg {
		_, err := gradle.RunTask(gradlew, task)
		return buildCompleteMsg{module: task, err: err}
	}
}

const headerLines = 2 // header + search bar above panels

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}

	// Adjust for lines above the panels
	mx, my := msg.X, msg.Y-headerLines

	// Find which panel was hit
	var hitPanel types.PanelID = -1
	for pid, region := range m.panelRegions {
		if region.Contains(mx, my) {
			hitPanel = pid
			break
		}
	}
	if hitPanel < 0 {
		return m, nil
	}

	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.state.Focus = hitPanel
		m.scrollPanel(hitPanel, -3)
		return m, nil

	case tea.MouseButtonWheelDown:
		m.state.Focus = hitPanel
		m.scrollPanel(hitPanel, 3)
		return m, nil

	case tea.MouseButtonLeft:
		m.state.Focus = hitPanel
		region := m.panelRegions[hitPanel]
		contentRow := my - region.Y - 1 // -1 for top border
		if contentRow >= 0 {
			m.clickItem(hitPanel, contentRow)
		}
	}
	return m, nil
}

// scrollPanel moves the cursor in the given panel by delta items.
func (m *Model) scrollPanel(pid types.PanelID, delta int) {
	switch pid {
	case types.PanelModules:
		m.state.ModuleSel = clamp(m.state.ModuleSel+delta, 0, max(0, len(m.modules)-1))
		m.state.PreviewSel = 0
	case types.PanelPreviews:
		previews := m.filteredPreviews()
		m.state.PreviewSel = clamp(m.state.PreviewSel+delta, 0, max(0, len(previews)-1))
	}
}

// clickItem selects the item at the given content row within a panel.
func (m *Model) clickItem(pid types.PanelID, row int) {
	switch pid {
	case types.PanelModules:
		region := m.panelRegions[pid]
		maxLines := region.H - 2 // subtract borders
		start, _ := visibleRange(m.state.ModuleSel, len(m.modules), maxLines, 1)
		idx := start + row
		if idx >= 0 && idx < len(m.modules) {
			m.state.ModuleSel = idx
			m.state.PreviewSel = 0
		}
	case types.PanelPreviews:
		previews := m.filteredPreviews()
		region := m.panelRegions[pid]
		maxLines := region.H - 2
		start, _ := visibleRange(m.state.PreviewSel, len(previews), maxLines, 1)
		idx := start + row
		if idx >= 0 && idx < len(previews) {
			m.state.PreviewSel = idx
		}
	}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
