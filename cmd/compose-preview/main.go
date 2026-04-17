package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ignaciotcrespo/compose-preview-cli/internal/adb"
	"github.com/ignaciotcrespo/compose-preview-cli/internal/gradle"
	"github.com/ignaciotcrespo/compose-preview-cli/internal/scanner"
	"github.com/ignaciotcrespo/compose-preview-cli/internal/server"
	"github.com/ignaciotcrespo/compose-preview-cli/internal/ui"
)

// version is set by goreleaser via ldflags.
var version = "dev"

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println("compose-preview", version)
		return
	}

	// Check for flags
	webMode := false
	listMode := false
	runPreview := ""
	screenshotPreview := ""
	screenshotOutput := "preview.png"
	screenshotDelay := 3
	webPort := 9999
	args := []string{}
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if arg == "--web" || arg == "-w" {
			webMode = true
		} else if arg == "--list" || arg == "-l" {
			listMode = true
		} else if (arg == "--run" || arg == "-r") && i+1 < len(os.Args) {
			i++
			runPreview = os.Args[i]
		} else if arg == "--screenshot" && i+1 < len(os.Args) {
			i++
			screenshotPreview = os.Args[i]
		} else if arg == "--output" && i+1 < len(os.Args) {
			i++
			screenshotOutput = os.Args[i]
		} else if arg == "--delay" && i+1 < len(os.Args) {
			i++
			d, err := strconv.Atoi(os.Args[i])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: invalid delay %q\n", os.Args[i])
				os.Exit(1)
			}
			screenshotDelay = d
		} else if (arg == "--port" || arg == "-p") && i+1 < len(os.Args) {
			i++
			p, err := strconv.Atoi(os.Args[i])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: invalid port %q\n", os.Args[i])
				os.Exit(1)
			}
			webPort = p
		} else {
			args = append(args, arg)
		}
	}

	// Determine project root
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	root, err := gradle.FindProjectRoot(absDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "Run this command from an Android/Gradle project directory, or pass the path as an argument.\n")
		os.Exit(1)
	}

	if webMode {
		runWebMode(root, webPort)
		return
	}

	if listMode {
		runListMode(root)
		return
	}

	if runPreview != "" {
		runPreviewMode(root, runPreview)
		return
	}

	if screenshotPreview != "" {
		runScreenshotMode(root, screenshotPreview, screenshotOutput, screenshotDelay)
		return
	}

	// Scan for previews
	fmt.Fprintf(os.Stderr, "Scanning %s for @Preview composables...\n", root)
	result := scanner.Scan(root)
	fmt.Fprintf(os.Stderr, "Found %d previews across %d modules\n", len(result.AllPreviews), len(result.Modules))

	if len(result.AllPreviews) == 0 {
		fmt.Fprintf(os.Stderr, "No @Preview composables found.\n")
		os.Exit(0)
	}

	// Launch TUI
	model := ui.NewModel(result, root)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithReportFocus())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runWebMode(root string, port int) {
	// In web mode, start the server and open the browser.
	// The server spawns the TUI in a PTY internally.
	goBinary, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	srv := server.New(port, goBinary, root)
	actualPort, err := srv.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting server: %v\n", err)
		os.Exit(1)
	}

	url := fmt.Sprintf("http://localhost:%d", actualPort)
	fmt.Fprintf(os.Stderr, "Compose Preview running at %s\n", url)

	// Open browser
	switch runtime.GOOS {
	case "darwin":
		exec.Command("open", url).Start()
	case "windows":
		exec.Command("cmd", "/c", "start", url).Start()
	default:
		exec.Command("xdg-open", url).Start()
	}

	// Keep running until Ctrl+C
	fmt.Fprintf(os.Stderr, "Press Ctrl+C to stop\n")
	select {}
}

func runListMode(root string) {
	result := scanner.Scan(root)

	type previewEntry struct {
		Module   string `json:"module"`
		Function string `json:"function"`
		FQN      string `json:"fqn"`
		File     string `json:"file"`
		Line     int    `json:"line"`
		Name     string `json:"name,omitempty"`
	}

	var entries []previewEntry
	for _, p := range result.AllPreviews {
		rel, _ := filepath.Rel(root, p.FilePath)
		entries = append(entries, previewEntry{
			Module:   p.Module,
			Function: p.FunctionName,
			FQN:      p.FQN,
			File:     rel,
			Line:     p.LineNumber,
			Name:     p.PreviewName,
		})
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(entries)
}

// resolvedPreview holds everything needed to launch a preview on a device.
type resolvedPreview struct {
	preview  scanner.PreviewFunc
	serial   string
	packages []string
}

// resolvePreview scans the project, finds the matching preview, device, and installed app.
func resolvePreview(root, query string) resolvedPreview {
	result := scanner.Scan(root)
	if len(result.AllPreviews) == 0 {
		fmt.Fprintf(os.Stderr, "No @Preview composables found.\n")
		os.Exit(1)
	}

	var match *scanner.PreviewFunc
	var partialMatches []scanner.PreviewFunc
	queryLower := strings.ToLower(query)
	for i, p := range result.AllPreviews {
		if p.FQN == query || p.FunctionName == query {
			match = &result.AllPreviews[i]
			break
		}
		if strings.Contains(strings.ToLower(p.FunctionName), queryLower) {
			partialMatches = append(partialMatches, p)
		}
	}
	if match == nil && len(partialMatches) == 1 {
		match = &partialMatches[0]
	}
	if match == nil {
		if len(partialMatches) > 1 {
			fmt.Fprintf(os.Stderr, "Multiple previews match %q:\n", query)
			for _, p := range partialMatches {
				fmt.Fprintf(os.Stderr, "  %s  (%s)\n", p.FunctionName, p.FQN)
			}
		} else {
			fmt.Fprintf(os.Stderr, "No preview found matching %q\n", query)
		}
		os.Exit(1)
	}

	devices, err := adb.DetectDevices()
	if err != nil || len(devices) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no device connected\n")
		os.Exit(1)
	}
	serial := devices[0].Serial

	appId, _ := findAppApplicationId(result.Modules, root)
	if appId == "" {
		fmt.Fprintf(os.Stderr, "Error: could not detect applicationId — check build.gradle.kts\n")
		os.Exit(1)
	}

	packages := adb.FindInstalledPackage(serial, appId)
	if len(packages) == 0 {
		fmt.Fprintf(os.Stderr, "Error: app not installed on %s\n", serial)
		fmt.Fprintf(os.Stderr, "Install it first:\n")
		fmt.Fprintf(os.Stderr, "  compose-preview          (TUI, press 'i')\n")
		fmt.Fprintf(os.Stderr, "  ./gradlew installDebug   (manual)\n")
		os.Exit(1)
	}

	return resolvedPreview{preview: *match, serial: serial, packages: packages}
}

// launchPreview launches the preview on the device, trying all installed variants.
func launchPreview(r resolvedPreview) {
	fmt.Fprintf(os.Stderr, "Launching %s on %s...\n", r.preview.FunctionName, r.serial)
	for _, pkg := range r.packages {
		if err := adb.LaunchPreview(r.serial, pkg, r.preview.FQN); err == nil {
			fmt.Fprintf(os.Stderr, "Launched: %s (%s)\n", r.preview.FunctionName, pkg)
			return
		}
	}
	fmt.Fprintf(os.Stderr, "Error: failed to launch preview with any installed variant\n")
	os.Exit(1)
}

func runPreviewMode(root, query string) {
	r := resolvePreview(root, query)
	launchPreview(r)
}

func runScreenshotMode(root, query, output string, delay int) {
	r := resolvePreview(root, query)
	launchPreview(r)

	fmt.Fprintf(os.Stderr, "Waiting %ds for preview to render...\n", delay)
	time.Sleep(time.Duration(delay) * time.Second)

	png, err := adb.CaptureScreenshot(r.serial)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error capturing screenshot: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(output, png, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", output, err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Screenshot saved to %s\n", output)
}

func findAppApplicationId(modules []scanner.Module, projectRoot string) (string, string) {
	for _, name := range []string{":composeApp", ":app"} {
		for _, mod := range modules {
			if mod.Name == name {
				if id := gradle.FindApplicationId(mod.Path); id != "" {
					return id, mod.Path
				}
			}
		}
	}
	for _, mod := range modules {
		if id := gradle.FindApplicationId(mod.Path); id != "" {
			return id, mod.Path
		}
	}
	return "", ""
}
