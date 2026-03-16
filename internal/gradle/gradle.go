package gradle

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
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

// RunTask runs an arbitrary gradle task.
func RunTask(gradlewPath, task string) (string, error) {
	cmd := exec.Command(gradlewPath, task)
	cmd.Dir = filepath.Dir(gradlewPath)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("gradle %s: %w", task, err)
	}
	return string(out), nil
}

// ListInstallTasks queries gradle for all install* tasks in a module.
// Returns task names like "installDevDebug", "installQaDebug", etc.
func ListInstallTasks(gradlewPath, moduleName string) []string {
	prefix := moduleName + ":"
	if moduleName == ":" {
		prefix = ""
	}

	cmd := exec.Command(gradlewPath, prefix+"tasks", "--all", "-q")
	cmd.Dir = filepath.Dir(gradlewPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Fallback: return just installDebug
		return []string{prefix + "installDebug"}
	}

	var tasks []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		// Match lines like "installDevDebug" or "composeApp:installAcceptDebug"
		// Strip the module prefix if present for matching
		taskName := line
		if strings.Contains(taskName, " - ") {
			taskName = strings.SplitN(taskName, " - ", 2)[0]
			taskName = strings.TrimSpace(taskName)
		}
		// We want full task names starting with install and containing Debug
		bare := strings.TrimPrefix(taskName, prefix)
		if strings.HasPrefix(bare, "install") && strings.Contains(bare, "Debug") {
			tasks = append(tasks, prefix+bare)
		}
	}

	if len(tasks) == 0 {
		return []string{prefix + "installDebug"}
	}

	// Sort: shorter names first (installDebug before installDevDebug)
	sortByLen(tasks)
	return tasks
}

func sortByLen(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && len(s[j]) < len(s[j-1]); j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
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
// Checks multiple flavor directories (debug, devDebug, qaDebug, etc.)
func FindDebugAPK(modulePath string) string {
	apkRoot := filepath.Join(modulePath, "build", "outputs", "apk")
	// Walk one or two levels to find any .apk file
	var newest string
	var newestTime time.Time
	filepath.Walk(apkRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.HasSuffix(info.Name(), ".apk") {
			if info.ModTime().After(newestTime) {
				newest = path
				newestTime = info.ModTime()
			}
		}
		return nil
	})
	return newest
}

// NewestSourceTime returns the modification time of the newest source file
// across all modules' src/ directories. Only checks .kt, .java, and .xml files.
func NewestSourceTime(projectRoot string) time.Time {
	var newest time.Time
	filepath.Walk(projectRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".gradle" || name == ".idea" || name == "build" || name == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(info.Name())
		if ext == ".kt" || ext == ".java" || ext == ".xml" {
			if info.ModTime().After(newest) {
				newest = info.ModTime()
			}
		}
		return nil
	})
	return newest
}

// NeedsBuild checks if source files are newer than the debug APK.
// Returns true if a rebuild is needed, along with a reason string.
func NeedsBuild(appModulePath, projectRoot string) (bool, string) {
	apk := FindDebugAPK(appModulePath)
	if apk == "" {
		return true, "no APK found — never built"
	}
	apkInfo, err := os.Stat(apk)
	if err != nil {
		return true, "no APK found — never built"
	}

	srcTime := NewestSourceTime(projectRoot)
	if srcTime.IsZero() {
		return false, ""
	}

	if srcTime.After(apkInfo.ModTime()) {
		return true, "sources changed since last build"
	}
	return false, ""
}
