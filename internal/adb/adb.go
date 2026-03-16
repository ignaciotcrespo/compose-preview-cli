package adb

import (
	"fmt"
	"os/exec"
	"strings"
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

// LaunchPreview starts PreviewActivity on the device with the given composable FQN.
// It force-stops the app first to ensure the new preview is rendered fresh.
func LaunchPreview(serial, appPackage, composableFQN string) error {
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
	return nil
}
