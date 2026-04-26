package ui

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"path/filepath"
	"runtime"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ignaciotcrespo/compose-preview-cli/internal/adb"
	"github.com/ignaciotcrespo/compose-preview-cli/internal/controller"
	"github.com/ignaciotcrespo/compose-preview-cli/internal/gradle"
	"github.com/ignaciotcrespo/compose-preview-cli/internal/scanner"
	"github.com/ignaciotcrespo/compose-preview-cli/internal/types"
	"github.com/ignaciotcrespo/compose-preview-cli/internal/server"
	"github.com/ignaciotcrespo/compose-preview-cli/internal/ui/imgrender"
	"github.com/ignaciotcrespo/compose-preview-cli/internal/ui/panel"
	"github.com/ignaciotcrespo/compose-preview-cli/internal/ui/prompt"
	"github.com/ignaciotcrespo/compose-preview-cli/internal/ui/screenshot"
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

// screenshotMsg is sent when a screenshot capture completes.
type screenshotMsg struct {
	fqn     string
	pngData []byte
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

// emulatorKilledMsg is sent when an emulator has been killed.
type emulatorKilledMsg struct {
	serial string
	err    error
}

// installTasksMsg is sent when gradle install task discovery completes.
type installTasksMsg struct {
	tasks []string
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
	webServer   *server.Server // nil until started by 'w'

	// Device / Emulator
	deviceStatus   string
	devices        []adb.Device
	avds           []adb.AVD  // available emulator AVDs
	showDevicePicker bool     // modal is visible
	devicePickerSel  int      // cursor in the picker
	devicePickerItems []devicePickerItem // combined list of devices + AVDs
	emulatorBooting  bool
	emulatorFastMode bool // headless + Quick Boot
	showKillPicker     bool                // kill emulator modal
	killPickerSel      int                 // cursor in the kill picker
	killPickerItems    []adb.RunningEmulator // running emulators

	// Build status
	statusMsg     string
	errorMsg      string
	building      bool
	needsBuild    bool
	buildWarning  string   // e.g. "sources changed since last build"
	appModulePath string   // path to the app module (for APK staleness check)
	installTasks      []string // cached install tasks (e.g. installDevDebug, installAcceptDebug)
	lastBuildTask     string   // remember last selected task
	showInstallPicker bool     // modal is visible
	installPickerSel  int      // cursor in the picker

	// Screenshot cache and rendered preview
	screenshotCache *screenshot.Cache
	renderedPreview string // current rendered half-block image
	previewErrors   map[string]string // FQN → crash error message
	previewFQN      string // FQN of the currently rendered preview
	capturing       bool   // screenshot capture in progress
	dismissDialog   bool   // send key events to dismiss compatibility dialog
	screenshotDelay time.Duration // wait time before capturing screenshot

	// Search bar (always visible at top)
	searchInput  textinput.Model
	searchActive bool // true when search bar is focused

	// Prompt
	prompt prompt.Prompt

	// Web mode: hide screenshot panel, browser shows HD preview
	electronMode bool

	// Panel regions for mouse click detection
	panelRegions map[types.PanelID]panel.Region
}

// Options configures optional TUI behavior.
type Options struct {
	DismissDialog   bool          // send key events to dismiss compatibility dialog after launch
	ScreenshotDelay time.Duration // wait time before capturing screenshot (default 3s)
}

// NewModel creates the initial model from scan results.
func NewModel(result scanner.ScanResult, projectRoot string, opts ...Options) Model {
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

	var opt Options
	if len(opts) > 0 {
		opt = opts[0]
	}
	if opt.ScreenshotDelay == 0 {
		opt.ScreenshotDelay = 1 * time.Second
	}

	m := Model{
		electronMode:     os.Getenv("COMPOSE_PREVIEW_ELECTRON") == "1",
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
		screenshotCache:  screenshot.NewCache(),
		previewErrors:    make(map[string]string),
		dismissDialog:    opt.DismissDialog,
		screenshotDelay:  opt.ScreenshotDelay,
		searchInput:      si,
		avds:             avds,
		showDevicePicker: showPicker,
		emulatorFastMode: true,
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

	case installTasksMsg:
		m.installTasks = msg.tasks
		if len(m.installTasks) == 0 {
			m.showInstallPicker = false
			m.errorMsg = "No install tasks found"
			return m, nil
		}
		if len(m.installTasks) == 1 {
			m.showInstallPicker = false
			return m, m.runBuildTask(m.installTasks[0])
		}
		// Tasks loaded — picker is already showing, it will now render the list
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

	case fullscreenDoneMsg:
		return m, nil

	case screenshotMsg:
		m.capturing = false
		if msg.err != nil {
			m.previewErrors[msg.fqn] = msg.err.Error()
		} else {
			delete(m.previewErrors, msg.fqn)
			m.screenshotCache.Put(msg.fqn, msg.pngData)
			m.previewFQN = msg.fqn
			// Render will pick it up from the cache
		}
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

	case emulatorKilledMsg:
		if msg.err != nil {
			m.errorMsg = fmt.Sprintf("Failed to kill emulator: %v", msg.err)
		} else {
			m.statusMsg = fmt.Sprintf("Emulator %s killed", msg.serial)
			// Remove killed device from list
			var remaining []adb.Device
			for _, d := range m.devices {
				if d.Serial != msg.serial {
					remaining = append(remaining, d)
				}
			}
			m.devices = remaining
			if len(m.devices) > 0 {
				m.deviceStatus = m.devices[0].Model
				if m.deviceStatus == "" {
					m.deviceStatus = m.devices[0].Serial
				}
			} else {
				m.deviceStatus = "no device"
			}
		}
		return m, nil

	case adbLaunchMsg:
		if msg.err != nil {
			m.errorMsg = fmt.Sprintf("Launch failed: %v", msg.err)
			return m, nil
		}
		m.statusMsg = fmt.Sprintf("Launched: %s", msg.preview)
		m.errorMsg = ""
		// Auto-capture screenshot after a short delay for the preview to render
		return m, m.delayedScreenshotCapture()

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

		// Kill emulator picker modal is active
		if m.showKillPicker {
			switch msg.String() {
			case "up", "k":
				if m.killPickerSel > 0 {
					m.killPickerSel--
				}
				return m, nil
			case "down", "j":
				if m.killPickerSel < len(m.killPickerItems)-1 {
					m.killPickerSel++
				}
				return m, nil
			case "enter":
				if m.killPickerSel < len(m.killPickerItems) {
					emu := m.killPickerItems[m.killPickerSel]
					m.showKillPicker = false
					serial := emu.Serial
					m.statusMsg = fmt.Sprintf("Killing emulator %s...", serial)
					return m, func() tea.Msg {
						err := adb.KillEmulator(serial)
						return emulatorKilledMsg{serial: serial, err: err}
					}
				}
				return m, nil
			case "esc", "q":
				m.showKillPicker = false
				return m, nil
			case "ctrl+c":
				return m, tea.Quit
			}
			return m, nil
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
			case "f":
				m.emulatorFastMode = !m.emulatorFastMode
				return m, nil
			case "esc", "q":
				m.showDevicePicker = false
				return m, nil
			case "ctrl+c":
				return m, tea.Quit
			}
			return m, nil
		}

		// Install task picker modal is active
		if m.showInstallPicker {
			switch msg.String() {
			case "up", "k":
				if m.installPickerSel > 0 {
					m.installPickerSel--
				}
				return m, nil
			case "down", "j":
				if m.installPickerSel < len(m.installTasks)-1 {
					m.installPickerSel++
				}
				return m, nil
			case "enter":
				if m.installPickerSel < len(m.installTasks) {
					task := m.installTasks[m.installPickerSel]
					m.showInstallPicker = false
					return m, m.runBuildTask(task)
				}
				return m, nil
			case "esc", "q":
				m.showInstallPicker = false
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

		// "x" opens kill emulator picker
		if key == "x" {
			emulators, err := adb.ListRunningEmulators()
			if err != nil || len(emulators) == 0 {
				m.errorMsg = "No running emulators found"
				return m, nil
			}
			m.killPickerItems = emulators
			m.killPickerSel = 0
			m.showKillPicker = true
			return m, nil
		}

		// "w" toggles web preview viewer
		if key == "w" {
			if m.webServer == nil {
				// Start server
				goBinary, _ := os.Executable()
				m.webServer = server.New(9999, goBinary, m.projectRoot)
				port, err := m.webServer.Start()
				if err != nil {
					m.errorMsg = fmt.Sprintf("Server error: %v", err)
					m.webServer = nil
				} else {
					url := fmt.Sprintf("http://localhost:%d", port)
					m.statusMsg = fmt.Sprintf("Web viewer: %s", url)
					// Open browser
					var cmd *exec.Cmd
					switch runtime.GOOS {
					case "darwin":
						cmd = exec.Command("open", url)
					case "windows":
						cmd = exec.Command("cmd", "/c", "start", url)
					default:
						cmd = exec.Command("xdg-open", url)
					}
					cmd.Start()
				}
			} else {
				// Stop server
				m.webServer.Stop()
				m.webServer = nil
				m.statusMsg = "Web viewer stopped"
			}
			return m, nil
		}

		// "s" captures screenshot of current preview
		if key == "s" {
			return m, m.captureScreenshot()
		}

		// "o" opens the cached screenshot in the system image viewer
		if key == "o" {
			m.openScreenshotExternal()
			return m, nil
		}

		// "f" shows the screenshot fullscreen using the terminal's native graphics protocol
		if key == "f" {
			return m, m.fullscreenPreview()
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

		// Signal selection change to external viewers
		if previews := m.filteredPreviews(); len(previews) > 0 && m.state.PreviewSel < len(previews) {
			m.screenshotCache.SignalSelection(previews[m.state.PreviewSel].FQN)
		}

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
				err:     fmt.Errorf("app not installed — build and install first (press 'i')"),
			}
		}

		// Clear logcat before launching so we can detect crashes afterwards
		adb.ClearLogcat(serial)

		// Try each installed variant until one works
		var lastErr error
		for _, pkg := range packages {
			err := adb.LaunchPreview(serial, pkg, fqn, m.dismissDialog)
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

	// If we used a task before, run it again directly
	if m.lastBuildTask != "" {
		return m.runBuildTask(m.lastBuildTask)
	}

	// Show picker modal immediately
	m.installPickerSel = 0
	m.showInstallPicker = true
	m.errorMsg = ""

	// If tasks are already cached, check shortcuts
	if len(m.installTasks) == 1 {
		m.showInstallPicker = false
		return m.runBuildTask(m.installTasks[0])
	}
	if len(m.installTasks) > 1 {
		return nil
	}

	// Discover install tasks asynchronously
	return func() tea.Msg {
		tasks := gradle.ListInstallTasks(gradlew, appModuleName)
		return installTasksMsg{tasks: tasks}
	}
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

// wordWrap breaks long lines at the given width, preserving existing newlines.
func wordWrap(s string, width int) string {
	if width <= 0 {
		return s
	}
	var result strings.Builder
	for _, line := range strings.Split(s, "\n") {
		for len(line) > width {
			result.WriteString("  " + line[:width] + "\n")
			line = line[width:]
		}
		result.WriteString("  " + line + "\n")
	}
	return strings.TrimRight(result.String(), "\n")
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

// openScreenshotExternal opens the cached screenshot in the system image viewer.
func (m *Model) openScreenshotExternal() {
	previews := m.filteredPreviews()
	if m.state.PreviewSel >= len(previews) {
		return
	}
	fqn := previews[m.state.PreviewSel].FQN
	entry := m.screenshotCache.Get(fqn)
	if entry == nil {
		m.errorMsg = "No screenshot cached — press 's' first"
		return
	}

	// Write to temp file
	tmpDir := filepath.Join(os.TempDir(), "compose-preview")
	os.MkdirAll(tmpDir, 0755)
	tmpFile := filepath.Join(tmpDir, "preview.png")
	if err := os.WriteFile(tmpFile, entry.PNGData, 0644); err != nil {
		m.errorMsg = fmt.Sprintf("Failed to write screenshot: %v", err)
		return
	}

	// Open with system viewer
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", tmpFile)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", tmpFile)
	default:
		cmd = exec.Command("xdg-open", tmpFile)
	}
	cmd.Start()
	m.statusMsg = fmt.Sprintf("Opened: %s", tmpFile)
}

// fullscreenPreview temporarily exits the TUI and shows the screenshot
// fullscreen using the terminal's native graphics protocol.
// Supports navigating between previews with up/down arrow keys.
func (m *Model) fullscreenPreview() tea.Cmd {
	previews := m.filteredPreviews()
	if len(previews) == 0 {
		return nil
	}
	return tea.Exec(&fullscreenImgCmd{
		previews:  previews,
		sel:       m.state.PreviewSel,
		cache:     m.screenshotCache,
		serial:    m.deviceSerial(),
		appId:     m.appId,
		delay:     m.screenshotDelay,
	}, func(err error) tea.Msg {
		return fullscreenDoneMsg{}
	})
}

// deviceSerial returns the serial of the first connected device, or empty.
func (m *Model) deviceSerial() string {
	if len(m.devices) > 0 {
		return m.devices[0].Serial
	}
	return ""
}

// fullscreenDoneMsg is sent when the fullscreen preview is dismissed.
type fullscreenDoneMsg struct{}

// fullscreenImgCmd implements tea.ExecCommand to display an image fullscreen
// with interactive navigation between previews.
type fullscreenImgCmd struct {
	previews []scanner.PreviewFunc
	sel      int
	cache    *screenshot.Cache
	serial   string
	appId    string
	delay    time.Duration
	stdin    io.Reader
	stdout   io.Writer
	stderr   io.Writer
}

func (c *fullscreenImgCmd) SetStdin(r io.Reader)  { c.stdin = r }
func (c *fullscreenImgCmd) SetStdout(w io.Writer)  { c.stdout = w }
func (c *fullscreenImgCmd) SetStderr(w io.Writer)  { c.stderr = w }

func (c *fullscreenImgCmd) Run() error {
	f, ok := c.stdin.(*os.File)
	if !ok {
		return nil
	}
	oldState, err := makeRaw(f.Fd())
	if err != nil {
		return nil
	}
	defer restoreTerminal(f.Fd(), oldState)

	proto := imgrender.DetectGraphics()
	sel := c.sel

	for {
		c.renderFullscreen(proto, sel)

		// Read key input
		key := readKey(f)
		switch key {
		case "up":
			if sel > 0 {
				sel--
			}
		case "down":
			if sel < len(c.previews)-1 {
				sel++
			}
		default:
			// Any other key exits fullscreen
			return nil
		}
	}
}

func (c *fullscreenImgCmd) renderFullscreen(proto imgrender.Protocol, sel int) {
	w, h := imgrender.TerminalSize(c.stdout)
	p := c.previews[sel]

	// Clear screen
	fmt.Fprint(c.stdout, "\033[2J\033[H")

	// Title bar
	title := fmt.Sprintf(" %s (%d/%d) — ↑↓ navigate · any key to return",
		p.FunctionName, sel+1, len(c.previews))
	fmt.Fprintln(c.stdout, title)
	fmt.Fprintln(c.stdout)

	// Check cache
	entry := c.cache.Get(p.FQN)
	if entry != nil {
		rendered := proto.Render(entry.PNGData, w, h-3)
		fmt.Fprint(c.stdout, rendered)
		return
	}

	// Not cached — try to capture
	if c.serial == "" {
		fmt.Fprint(c.stdout, "  No device connected")
		return
	}

	fmt.Fprint(c.stdout, "  Capturing screenshot...")

	// Launch and capture
	packages := adb.FindInstalledPackage(c.serial, c.appId)
	for _, pkg := range packages {
		if err := adb.LaunchPreview(c.serial, pkg, p.FQN, false); err == nil {
			break
		}
	}
	time.Sleep(c.delay)
	data, err := adb.CaptureScreenshot(c.serial)
	if err != nil {
		fmt.Fprint(c.stdout, "\n  Capture failed: "+err.Error())
		return
	}

	c.cache.Put(p.FQN, data)

	// Clear and redraw with the image
	fmt.Fprint(c.stdout, "\033[2J\033[H")
	fmt.Fprintln(c.stdout, fmt.Sprintf(" %s (%d/%d) — ↑↓ navigate · any key to return",
		p.FunctionName, sel+1, len(c.previews)))
	fmt.Fprintln(c.stdout)
	rendered := proto.Render(data, w, h-3)
	fmt.Fprint(c.stdout, rendered)
}

// readKey reads a single key or escape sequence from the terminal in raw mode.
// Returns "up", "down", or the raw character string.
func readKey(f *os.File) string {
	buf := make([]byte, 3)
	n, err := f.Read(buf)
	if err != nil || n == 0 {
		return "q"
	}
	if n == 1 {
		return string(buf[0])
	}
	// Escape sequences: ESC [ A (up), ESC [ B (down)
	if n >= 3 && buf[0] == 0x1b && buf[1] == '[' {
		switch buf[2] {
		case 'A':
			return "up"
		case 'B':
			return "down"
		}
	}
	return string(buf[:n])
}

// makeRaw and restoreTerminal are in terminal_darwin.go / terminal_linux.go

// captureScreenshot takes a screenshot of the current preview.
func (m *Model) captureScreenshot() tea.Cmd {
	previews := m.filteredPreviews()
	if m.state.PreviewSel >= len(previews) || len(m.devices) == 0 {
		return nil
	}
	if m.capturing {
		return nil
	}
	m.capturing = true

	p := previews[m.state.PreviewSel]
	serial := m.devices[0].Serial
	fqn := p.FQN
	m.screenshotCache.SignalCapturing(fqn)

	return func() tea.Msg {
		data, err := adb.CaptureScreenshot(serial)
		return screenshotMsg{fqn: fqn, pngData: data, err: err}
	}
}

// delayedScreenshotCapture waits a moment then captures a screenshot.
func (m *Model) delayedScreenshotCapture() tea.Cmd {
	previews := m.filteredPreviews()
	if m.state.PreviewSel >= len(previews) || len(m.devices) == 0 {
		return nil
	}
	m.capturing = true

	p := previews[m.state.PreviewSel]
	serial := m.devices[0].Serial
	fqn := p.FQN
	m.screenshotCache.SignalCapturing(fqn)

	delay := m.screenshotDelay
	return func() tea.Msg {
		// Wait for the preview to render (logcat was cleared before launch)
		time.Sleep(delay)

		// Check logcat for crashes (no extra wait, crash already happened if it will)
		crash := adb.CheckPreviewCrash(serial)
		if crash != "" {
			return screenshotMsg{fqn: fqn, err: fmt.Errorf("%s", crash)}
		}
		data, err := adb.CaptureScreenshot(serial)
		return screenshotMsg{fqn: fqn, pngData: data, err: err}
	}
}

// currentPreviewScreenshot returns the rendered screenshot for the currently selected preview.
func (m Model) currentPreviewScreenshot(width, height int) (string, string) {
	previews := m.filteredPreviews()
	if m.state.PreviewSel >= len(previews) {
		return "", ""
	}

	fqn := previews[m.state.PreviewSel].FQN

	if m.capturing && fqn == m.previewFQN {
		return "  Capturing...", ""
	}

	// Show crash error in the preview panel if the preview failed
	if errMsg, ok := m.previewErrors[fqn]; ok {
		wrapped := wordWrap(errMsg, width-4)
		return errorStyle.Render("  Preview crashed:\n\n" + wrapped), ""
	}

	entry := m.screenshotCache.Get(fqn)
	if entry == nil {
		if m.capturing {
			return "  Capturing...", ""
		}
		return "  No screenshot (s to capture)", ""
	}

	rendered := imgrender.Render(entry.PNGData, width, height-2) // -1 age line, -1 hint line
	age := "cached " + entry.Age()
	return rendered, age
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
	fastMode := m.emulatorFastMode
	mode := ""
	if fastMode {
		mode = " (fast/headless)"
	}
	m.statusMsg = fmt.Sprintf("Launching emulator %s%s...", avdName, mode)
	m.errorMsg = ""
	return func() tea.Msg {
		err := adb.StartEmulator(avdName, fastMode)
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
