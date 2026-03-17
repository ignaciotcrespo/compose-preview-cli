package ui

import (
	"fmt"
	"time"

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

// emulatorStartedMsg is sent when an emulator launch begins.
type emulatorStartedMsg struct {
	avdName string
	err     error
}

// emulatorReadyMsg is sent when the emulator is ready (device detected).
type emulatorReadyMsg struct {
	device adb.Device
}

// devicePickerItem is an entry in the device/emulator picker.
type devicePickerItem struct {
	label    string
	isDevice bool   // true = connected device, false = AVD to launch
	serial   string // for devices
	avdName  string // for AVDs
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

	// Device / Emulator
	deviceStatus   string
	devices        []adb.Device
	avds           []adb.AVD  // available emulator AVDs
	showDevicePicker bool     // modal is visible
	devicePickerSel  int      // cursor in the picker
	devicePickerItems []devicePickerItem // combined list of devices + AVDs
	emulatorBooting  bool

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
	var avds []adb.AVD
	showPicker := false
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
			// No devices — check for emulators
			avds = adb.ListAVDs()
			if len(avds) > 0 {
				showPicker = true
			}
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

	m := Model{
		state:            controller.NewState(),
		scanResult:       result,
		modules:          modules,
		projectRoot:      projectRoot,
		appId:            appId,
		appModulePath:    appModulePath,
		devices:          devices,
		deviceStatus:     deviceStatus,
		needsBuild:       needsBuild,
		buildWarning:     buildWarning,
		searchInput:      si,
		avds:             avds,
		showDevicePicker: showPicker,
		panelRegions:     make(map[types.PanelID]panel.Region),
	}
	if showPicker {
		m.devicePickerItems = m.buildPickerItems()
	}
	return m
}

// scanCompleteMsg is sent when an async rescan finishes.
type scanCompleteMsg struct {
	result scanner.ScanResult
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

	case tea.FocusMsg:
		// Terminal window gained focus — refresh data
		return m, m.startRescan()

	case scanCompleteMsg:
		m.applyRescan(msg.result)
		if m.needsBuild {
			// Let the build warning show instead of the refresh message
			m.statusMsg = ""
		} else {
			m.statusMsg = fmt.Sprintf("Refreshed: %d previews across %d modules", len(msg.result.AllPreviews), len(msg.result.Modules))
		}
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

	case emulatorStartedMsg:
		if msg.err != nil {
			m.errorMsg = fmt.Sprintf("Failed to start emulator: %v", msg.err)
			m.emulatorBooting = false
		} else {
			m.statusMsg = fmt.Sprintf("Starting emulator %s...", msg.avdName)
			m.emulatorBooting = true
			m.errorMsg = ""
			// Poll for device to appear
			return m, m.pollForEmulator()
		}
		return m, nil

	case emulatorReadyMsg:
		m.emulatorBooting = false
		m.devices = append(m.devices, msg.device)
		if msg.device.Model != "" {
			m.deviceStatus = msg.device.Model
		} else {
			m.deviceStatus = msg.device.Serial
		}
		m.statusMsg = fmt.Sprintf("Emulator ready: %s", m.deviceStatus)
		m.errorMsg = ""
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

		// Device picker modal is active
		if m.showDevicePicker {
			switch msg.String() {
			case "up", "k":
				if m.devicePickerSel > 0 {
					m.devicePickerSel--
				}
				return m, nil
			case "down", "j":
				if m.devicePickerSel < len(m.devicePickerItems)-1 {
					m.devicePickerSel++
				}
				return m, nil
			case "enter":
				if m.devicePickerSel < len(m.devicePickerItems) {
					item := m.devicePickerItems[m.devicePickerSel]
					m.showDevicePicker = false
					if item.isDevice {
						// Select existing device
						for _, d := range m.devices {
							if d.Serial == item.serial {
								if d.Model != "" {
									m.deviceStatus = d.Model
								} else {
									m.deviceStatus = d.Serial
								}
								// Move selected device to front
								break
							}
						}
					} else {
						// Launch emulator
						return m, m.launchEmulator(item.avdName)
					}
				}
				return m, nil
			case "esc", "q":
				m.showDevicePicker = false
				return m, nil
			case "ctrl+c":
				return m, tea.Quit
			}
			return m, nil
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

		// "d" opens device/emulator picker
		if key == "d" {
			m.avds = adb.ListAVDs() // refresh AVD list
			if newDevices, err := adb.DetectDevices(); err == nil {
				m.devices = newDevices
			}
			m.devicePickerItems = m.buildPickerItems()
			m.devicePickerSel = 0
			m.showDevicePicker = true
			return m, nil
		}

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
		if kr.RunRefresh {
			return m, m.startRescan()
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

// buildPickerItems creates the combined list of connected devices and available AVDs.
func (m *Model) buildPickerItems() []devicePickerItem {
	var items []devicePickerItem
	// Connected devices first
	for _, d := range m.devices {
		label := d.Serial
		if d.Model != "" {
			label = d.Model + " (" + d.Serial + ")"
		}
		items = append(items, devicePickerItem{
			label:    label,
			isDevice: true,
			serial:   d.Serial,
		})
	}
	// Then AVDs
	for _, avd := range m.avds {
		items = append(items, devicePickerItem{
			label:   avd.Name,
			avdName: avd.Name,
		})
	}
	return items
}

// launchEmulator starts an emulator AVD asynchronously.
func (m *Model) launchEmulator(avdName string) tea.Cmd {
	m.statusMsg = fmt.Sprintf("Launching emulator %s...", avdName)
	m.errorMsg = ""
	return func() tea.Msg {
		err := adb.StartEmulator(avdName)
		return emulatorStartedMsg{avdName: avdName, err: err}
	}
}

// pollForEmulator polls adb devices until a new emulator appears.
func (m *Model) pollForEmulator() tea.Cmd {
	existingSerials := make(map[string]bool)
	for _, d := range m.devices {
		existingSerials[d.Serial] = true
	}
	return func() tea.Msg {
		// Poll every 2 seconds for up to 60 seconds
		for i := 0; i < 30; i++ {
			time.Sleep(2 * time.Second)
			devices, err := adb.DetectDevices()
			if err != nil {
				continue
			}
			for _, d := range devices {
				if !existingSerials[d.Serial] && d.State == "device" {
					return emulatorReadyMsg{device: d}
				}
			}
		}
		return emulatorReadyMsg{device: adb.Device{Serial: "timeout", Model: "emulator (timeout)"}}
	}
}

// startRescan launches an async rescan of the project.
func (m *Model) startRescan() tea.Cmd {
	m.statusMsg = "Scanning..."
	m.errorMsg = ""
	root := m.projectRoot
	return func() tea.Msg {
		result := scanner.Scan(root)
		return scanCompleteMsg{result: result}
	}
}

// applyRescan updates the model with fresh scan results, preserving selection where possible.
func (m *Model) applyRescan(result scanner.ScanResult) {
	m.scanResult = result

	// Rebuild module list (only modules with previews)
	var modulesWithPreviews []scanner.Module
	for _, mod := range result.Modules {
		if len(mod.Previews) > 0 {
			modulesWithPreviews = append(modulesWithPreviews, mod)
		}
	}
	if len(modulesWithPreviews) > 0 {
		m.modules = modulesWithPreviews
	} else {
		m.modules = result.Modules
	}

	// Clamp selections
	if m.state.ModuleSel >= len(m.modules) {
		m.state.ModuleSel = max(0, len(m.modules)-1)
	}
	previews := m.filteredPreviews()
	if m.state.PreviewSel >= len(previews) {
		m.state.PreviewSel = max(0, len(previews)-1)
	}

	// Refresh applicationId and build staleness
	m.appId, m.appModulePath = findAppApplicationId(result.Modules, m.projectRoot)
	m.needsBuild, m.buildWarning = gradle.NeedsBuild(m.appModulePath, m.projectRoot)

	// Refresh devices
	if adb.IsADBAvailable() {
		if d, err := adb.DetectDevices(); err == nil && len(d) > 0 {
			m.devices = d
			if d[0].Model != "" {
				m.deviceStatus = d[0].Model
			} else {
				m.deviceStatus = d[0].Serial
			}
		} else {
			m.devices = nil
			m.deviceStatus = "no device"
		}
	}
}
