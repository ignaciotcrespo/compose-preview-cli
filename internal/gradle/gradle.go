package gradle

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	applicationIdRe = regexp.MustCompile(`applicationId\s*(?:=\s*|)"([^"]+)"`)
	namespaceRe     = regexp.MustCompile(`namespace\s*(?:=\s*|)"([^"]+)"`)
)

// FindGradlew locates the gradlew script in the project root.
func FindGradlew(root string) (string, error) {
	path := filepath.Join(root, "gradlew")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	path = filepath.Join(root, "gradlew.bat")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("gradlew not found in %s", root)
}

// FindApplicationId parses build.gradle(.kts) for the applicationId or namespace.
// It checks applicationId first, then falls back to namespace (AGP 7+).
func FindApplicationId(modulePath string) string {
	for _, name := range []string{"build.gradle.kts", "build.gradle"} {
		path := filepath.Join(modulePath, name)
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		defer f.Close()
		var namespace string
		s := bufio.NewScanner(f)
		for s.Scan() {
			line := s.Text()
			if m := applicationIdRe.FindStringSubmatch(line); m != nil {
				return m[1]
			}
			if namespace == "" {
				if m := namespaceRe.FindStringSubmatch(line); m != nil {
					namespace = m[1]
				}
			}
		}
		if namespace != "" {
			return namespace
		}
	}
	return ""
}

// AssembleDebug runs the gradle assembleDebug task for a module.
// Returns the combined output and any error.
func AssembleDebug(gradlewPath, moduleName string) (string, error) {
	task := moduleName + ":assembleDebug"
	if moduleName == ":" {
		task = "assembleDebug"
	}
	cmd := exec.Command(gradlewPath, task)
	cmd.Dir = filepath.Dir(gradlewPath)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("gradle %s: %w", task, err)
	}
	return string(out), nil
}

// InstallDebug runs the gradle installDebug task for a module.
func InstallDebug(gradlewPath, moduleName string) (string, error) {
	task := moduleName + ":installDebug"
	if moduleName == ":" {
		task = "installDebug"
	}
	cmd := exec.Command(gradlewPath, task)
	cmd.Dir = filepath.Dir(gradlewPath)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("gradle %s: %w", task, err)
	}
	return string(out), nil
}

// FindProjectRoot walks up from the given path to find the directory containing settings.gradle(.kts).
func FindProjectRoot(startDir string) (string, error) {
	dir := startDir
	for {
		for _, name := range []string{"settings.gradle.kts", "settings.gradle"} {
			if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	// Fallback: look for build.gradle at startDir
	for _, name := range []string{"build.gradle.kts", "build.gradle"} {
		if _, err := os.Stat(filepath.Join(startDir, name)); err == nil {
			return startDir, nil
		}
	}
	return "", fmt.Errorf("not an Android/Gradle project: %s", startDir)
}

// DetectModulesFromSettings parses settings.gradle(.kts) for include() directives.
func DetectModulesFromSettings(root string) []string {
	includeRe := regexp.MustCompile(`include\s*\(\s*"([^"]+)"`)
	var modules []string

	for _, name := range []string{"settings.gradle.kts", "settings.gradle"} {
		path := filepath.Join(root, name)
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			matches := includeRe.FindAllStringSubmatch(scanner.Text(), -1)
			for _, m := range matches {
				modules = append(modules, m[1])
			}
		}
	}
	return modules
}

// FindDebugAPK looks for the debug APK in the module's build output.
func FindDebugAPK(modulePath string) string {
	apkDir := filepath.Join(modulePath, "build", "outputs", "apk", "debug")
	entries, err := os.ReadDir(apkDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".apk") {
			return filepath.Join(apkDir, e.Name())
		}
	}
	return ""
}
