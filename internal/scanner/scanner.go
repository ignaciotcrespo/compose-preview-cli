package scanner

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// NOTE: path/filepath is used by both discoverModules/scanModule and jvmClassName

var (
	packageRe     = regexp.MustCompile(`^package\s+([\w.]+)`)
	previewRe     = regexp.MustCompile(`^\s*@Preview`)
	previewNameRe = regexp.MustCompile(`name\s*=\s*"([^"]*)"`)
	paramRe       = regexp.MustCompile(`(\w+)\s*=\s*("?[^,)"]*"?)`)
	funRe         = regexp.MustCompile(`^\s*(?:private\s+|internal\s+)?fun\s+(\w+)\s*\(`)
	composableRe  = regexp.MustCompile(`^\s*@Composable`)
)

// Scan walks the project root, discovers gradle modules, and extracts @Preview functions.
func Scan(root string) ScanResult {
	projectName := filepath.Base(root)
	modules := discoverModules(root)

	var allPreviews []PreviewFunc
	for i := range modules {
		previews, count := scanModule(modules[i].Path, modules[i].Name)
		modules[i].Previews = previews
		modules[i].ComposableCount = count
		allPreviews = append(allPreviews, previews...)
	}

	return ScanResult{
		Modules:     modules,
		AllPreviews: allPreviews,
		ProjectName: projectName,
	}
}

// discoverModules finds gradle modules by looking for build.gradle(.kts) files.
func discoverModules(root string) []Module {
	var modules []Module
	seen := make(map[string]bool)

	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
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
		base := info.Name()
		if base == "build.gradle" || base == "build.gradle.kts" {
			dir := filepath.Dir(path)
			if seen[dir] {
				return nil
			}
			seen[dir] = true

			rel, _ := filepath.Rel(root, dir)
			var moduleName string
			if rel == "." {
				moduleName = ":"
			} else {
				moduleName = ":" + strings.ReplaceAll(rel, string(filepath.Separator), ":")
			}
			modules = append(modules, Module{
				Name: moduleName,
				Path: dir,
			})
		}
		return nil
	})
	return modules
}

// scanModule scans all .kt files in a module's src directory for @Preview functions.
// Returns previews found and total composable count.
func scanModule(modulePath, moduleName string) ([]PreviewFunc, int) {
	srcDir := filepath.Join(modulePath, "src")
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		return nil, 0
	}

	var previews []PreviewFunc
	composableCount := 0
	filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".kt") {
			return nil
		}
		found, count := scanFile(path, moduleName)
		previews = append(previews, found...)
		composableCount += count
		return nil
	})
	return previews, composableCount
}

// jvmClassName returns the JVM class name for top-level functions in a Kotlin file.
// e.g. "LoginComponentsPreview.kt" → "LoginComponentsPreviewKt"
func jvmClassName(filePath string) string {
	base := filepath.Base(filePath)
	name := strings.TrimSuffix(base, ".kt")
	return name + "Kt"
}

// scanFile parses a single Kotlin file for @Preview annotated functions.
// Returns previews found and total @Composable function count.
func scanFile(path, moduleName string) ([]PreviewFunc, int) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0
	}
	defer f.Close()

	className := jvmClassName(path)

	var (
		previews        []PreviewFunc
		composableCount int
		pkg             string
		inPreview       bool
		inComposable    bool
		previewLine     int
		parenDepth      int
		annotationText  strings.Builder
	)

	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Track package
		if m := packageRe.FindStringSubmatch(line); m != nil {
			pkg = m[1]
			continue
		}

		// Detect @Preview annotation start
		if previewRe.MatchString(line) {
			inPreview = true
			previewLine = lineNum
			annotationText.Reset()
			annotationText.WriteString(line)
			// Count parens to handle multiline annotations
			parenDepth = strings.Count(line, "(") - strings.Count(line, ")")
			if parenDepth <= 0 {
				// Single line or no-param @Preview
				parenDepth = 0
			}
			continue
		}

		// Continue collecting multiline annotation
		if inPreview && parenDepth > 0 {
			annotationText.WriteString(line)
			parenDepth += strings.Count(line, "(") - strings.Count(line, ")")
			if parenDepth <= 0 {
				parenDepth = 0
			}
			continue
		}

		// Track @Composable annotations (for counting all composables)
		if composableRe.MatchString(line) {
			inComposable = true
			if inPreview {
				continue
			}
		}

		// Count @Composable functions (with or without @Preview)
		if !inPreview && inComposable {
			if funRe.MatchString(line) {
				composableCount++
				inComposable = false
			}
		}

		// Look for function declaration after @Preview
		if inPreview {
			if m := funRe.FindStringSubmatch(line); m != nil {
				composableCount++ // previews are also composables
				funcName := m[1]
				// FQN uses JVM class name: package.FileNameKt.FunctionName
				fqn := className + "." + funcName
				if pkg != "" {
					fqn = pkg + "." + className + "." + funcName
				}

				// Parse annotation params
				annText := annotationText.String()
				params := parseParams(annText)
				previewName := ""
				if n := previewNameRe.FindStringSubmatch(annText); n != nil {
					previewName = n[1]
				}

				previews = append(previews, PreviewFunc{
					Package:      pkg,
					FunctionName: funcName,
					FQN:          fqn,
					FilePath:     path,
					LineNumber:   previewLine,
					PreviewName:  previewName,
					Params:       params,
					Module:       moduleName,
				})
				inPreview = false
				continue
			}

			// If we hit a non-annotation, non-composable, non-fun line, cancel
			trimmed := strings.TrimSpace(line)
			if trimmed != "" && !strings.HasPrefix(trimmed, "@") && !strings.HasPrefix(trimmed, "//") {
				inPreview = false
			}
		}
	}
	return previews, composableCount
}

// parseParams extracts key=value pairs from an annotation string.
func parseParams(annotation string) map[string]string {
	params := make(map[string]string)
	matches := paramRe.FindAllStringSubmatch(annotation, -1)
	for _, m := range matches {
		key := m[1]
		val := strings.Trim(m[2], "\"")
		params[key] = val
	}
	return params
}
