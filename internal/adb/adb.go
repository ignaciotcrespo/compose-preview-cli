package adb

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Device represents a connected Android device.
type Device struct {
	Serial string
	Model  string
	State  string // "device", "offline", "unauthorized"
}

// IsADBAvailable checks if adb is on PATH.
func IsADBAvailable() bool {
	_, err := exec.LookPath("adb")
	return err == nil
}

// DetectDevices returns connected ADB devices.
func DetectDevices() ([]Device, error) {
	out, err := exec.Command("adb", "devices", "-l").Output()
	if err != nil {
		return nil, fmt.Errorf("adb devices: %w", err)
	}

	var devices []Device
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "List of") || strings.HasPrefix(line, "*") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		d := Device{
			Serial: parts[0],
			State:  parts[1],
		}
		// Extract model from "model:Pixel_6" if present
		for _, p := range parts[2:] {
			if strings.HasPrefix(p, "model:") {
				d.Model = strings.TrimPrefix(p, "model:")
				break
			}
		}
		devices = append(devices, d)
	}
	return devices, nil
}

// FindInstalledPackage searches for installed packages matching the base applicationId.
// Returns all matching packages (e.g. com.example.app, com.example.app.dev, com.example.app.debug).
func FindInstalledPackage(serial, baseAppId string) []string {
	out, err := exec.Command("adb", "-s", serial, "shell", "pm", "list", "packages").Output()
	if err != nil {
		return nil
	}
	var matches []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		pkg := strings.TrimPrefix(line, "package:")
		if pkg == baseAppId || strings.HasPrefix(pkg, baseAppId+".") {
			matches = append(matches, pkg)
		}
	}
	return matches
}

// AVD represents an Android Virtual Device (emulator).
type AVD struct {
	Name string
}

// ListAVDs returns available Android emulator AVDs.
func ListAVDs() []AVD {
	// Try to find emulator binary
	emulator, err := findEmulator()
	if err != nil {
		return nil
	}
	out, err := exec.Command(emulator, "-list-avds").Output()
	if err != nil {
		return nil
	}
	var avds []AVD
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			avds = append(avds, AVD{Name: line})
		}
	}
	return avds
}

// StartEmulator launches an emulator AVD in the background.
// Returns immediately — the emulator boots asynchronously.
func StartEmulator(avdName string) error {
	emulator, err := findEmulator()
	if err != nil {
		return err
	}
	cmd := exec.Command(emulator, "-avd", avdName, "-no-snapshot-load")
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Start()
}

// findEmulator locates the emulator binary.
func findEmulator() (string, error) {
	// Check PATH first
	if p, err := exec.LookPath("emulator"); err == nil {
		return p, nil
	}
	// Check ANDROID_HOME / ANDROID_SDK_ROOT
	for _, envVar := range []string{"ANDROID_HOME", "ANDROID_SDK_ROOT"} {
		sdk := os.Getenv(envVar)
		if sdk != "" {
			p := fmt.Sprintf("%s/emulator/emulator", sdk)
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
	}
	return "", fmt.Errorf("emulator not found — set ANDROID_HOME or add emulator to PATH")
}

// WaitForDevice waits for a device to come online (up to timeout).
func WaitForDevice(timeout int) error {
	args := []string{"wait-for-device"}
	cmd := exec.Command("adb", args...)
	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()
	select {
	case err := <-done:
		return err
	case <-make(chan struct{}):
		return fmt.Errorf("timeout waiting for device")
	}
}

// CaptureScreenshot takes a screenshot from the device and returns the PNG data.
func CaptureScreenshot(serial string) ([]byte, error) {
	cmd := exec.Command("adb", "-s", serial, "exec-out", "screencap", "-p")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("screencap: %w", err)
	}
	return out, nil
}

// LaunchPreview starts PreviewActivity on the device with the given composable FQN.
// It force-stops the app first to ensure the new preview is rendered fresh.
// If dismissDialog is true, it sends key events to dismiss the "built for older Android" dialog.
func LaunchPreview(serial, appPackage, composableFQN string, dismissDialog bool) error {
	// Force-stop the app so PreviewActivity restarts with the new composable
	exec.Command("adb", "-s", serial, "shell", "am", "force-stop", appPackage).Run()

	activity := appPackage + "/androidx.compose.ui.tooling.PreviewActivity"
	args := []string{
		"-s", serial,
		"shell", "am", "start",
		"-n", activity,
		"--es", "composable", composableFQN,
	}
	cmd := exec.Command("adb", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	// Check for "Error" in successful exit (adb returns 0 even on activity-not-found)
	outStr := string(out)
	if strings.Contains(outStr, "Error") {
		return fmt.Errorf("%s", strings.TrimSpace(outStr))
	}

	if dismissDialog {
		// Dismiss "built for an older version of Android" dialog if it appears.
		// Send two ENTERs: first focuses the OK button, second confirms it.
		go func() {
			time.Sleep(500 * time.Millisecond)
			exec.Command("adb", "-s", serial, "shell", "input", "keyevent", "KEYCODE_ENTER").Run()
			time.Sleep(200 * time.Millisecond)
			exec.Command("adb", "-s", serial, "shell", "input", "keyevent", "KEYCODE_ENTER").Run()
		}()
	}

	return nil
}

// CheckPreviewCrash clears logcat, waits for the given duration, then checks
// if the app crashed. Returns the crash message if found, or empty string.
func CheckPreviewCrash(serial, appPackage string, wait time.Duration) string {
	// Clear logcat before waiting
	exec.Command("adb", "-s", serial, "logcat", "-c").Run()

	time.Sleep(wait)

	// Read logcat for crashes — filter by AndroidRuntime (crash tag) and the app package
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "adb", "-s", serial, "logcat", "-d",
		"-s", "AndroidRuntime:E")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	logcat := string(out)
	if !strings.Contains(logcat, "FATAL EXCEPTION") {
		return ""
	}

	// Find the deepest "Caused by:" line — that's the root cause.
	// Also collect the line right after it (the first "at ..." gives context).
	lines := strings.Split(logcat, "\n")
	lastCausedBy := ""
	lastCausedByNext := ""
	for i, line := range lines {
		cleaned := stripLogcatPrefix(line)
		if strings.HasPrefix(cleaned, "Caused by:") {
			lastCausedBy = cleaned
			if i+1 < len(lines) {
				lastCausedByNext = strings.TrimSpace(stripLogcatPrefix(lines[i+1]))
			}
		}
	}

	if lastCausedBy == "" {
		// No "Caused by", use the main exception line
		for _, line := range lines {
			cleaned := stripLogcatPrefix(line)
			if strings.Contains(cleaned, "Exception") || strings.Contains(cleaned, "Error") {
				if !strings.Contains(cleaned, "FATAL EXCEPTION") && !strings.Contains(cleaned, "Process:") {
					lastCausedBy = cleaned
					break
				}
			}
		}
	}

	if lastCausedBy == "" {
		return ""
	}

	// Format: "ExceptionType: message"
	// Strip the "Caused by: " prefix for cleaner display
	result := strings.TrimPrefix(lastCausedBy, "Caused by: ")
	if lastCausedByNext != "" && strings.HasPrefix(lastCausedByNext, "at ") {
		result += "\n" + lastCausedByNext
	}
	return result
}

// stripLogcatPrefix removes the logcat metadata prefix from a line.
// e.g. "04-18 01:43:02.095  4918  4918 E AndroidRuntime: actual message"
// becomes "actual message"
func stripLogcatPrefix(line string) string {
	line = strings.TrimSpace(line)
	// Logcat format: "date time pid tid level tag: message"
	// The tag for crashes is "AndroidRuntime", look for that marker
	if idx := strings.Index(line, "AndroidRuntime:"); idx >= 0 {
		return strings.TrimSpace(line[idx+len("AndroidRuntime:"):])
	}
	return line
}
