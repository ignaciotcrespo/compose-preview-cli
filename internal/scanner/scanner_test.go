package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanFile_SimplePreview(t *testing.T) {
	dir := t.TempDir()
	kt := filepath.Join(dir, "MyScreen.kt")
	os.WriteFile(kt, []byte(`package com.example.myapp.ui

import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.runtime.Composable

@Preview
@Composable
fun MyButtonPreview() {
    MyButton()
}
`), 0644)

	previews := scanFile(kt, ":app")
	if len(previews) != 1 {
		t.Fatalf("expected 1 preview, got %d", len(previews))
	}
	p := previews[0]
	if p.FunctionName != "MyButtonPreview" {
		t.Errorf("expected MyButtonPreview, got %s", p.FunctionName)
	}
	if p.FQN != "com.example.myapp.ui.MyScreenKt.MyButtonPreview" {
		t.Errorf("expected com.example.myapp.ui.MyScreenKt.MyButtonPreview, got %s", p.FQN)
	}
	if p.Module != ":app" {
		t.Errorf("expected :app, got %s", p.Module)
	}
}

func TestScanFile_PreviewWithParams(t *testing.T) {
	dir := t.TempDir()
	kt := filepath.Join(dir, "MyScreen.kt")
	os.WriteFile(kt, []byte(`package com.example.ui

@Preview(name = "Dark Mode", showBackground = true, device = "pixel5")
@Composable
fun CardPreview() {
    Card()
}
`), 0644)

	previews := scanFile(kt, ":feature")
	if len(previews) != 1 {
		t.Fatalf("expected 1 preview, got %d", len(previews))
	}
	p := previews[0]
	if p.PreviewName != "Dark Mode" {
		t.Errorf("expected 'Dark Mode', got '%s'", p.PreviewName)
	}
	if p.Params["showBackground"] != "true" {
		t.Errorf("expected showBackground=true, got %s", p.Params["showBackground"])
	}
	if p.Params["device"] != "pixel5" {
		t.Errorf("expected device=pixel5, got %s", p.Params["device"])
	}
}

func TestScanFile_MultilineAnnotation(t *testing.T) {
	dir := t.TempDir()
	kt := filepath.Join(dir, "MyScreen.kt")
	os.WriteFile(kt, []byte(`package com.example

@Preview(
    name = "Large",
    widthDp = 400,
    heightDp = 800
)
@Composable
fun LargePreview() {
    Screen()
}
`), 0644)

	previews := scanFile(kt, ":app")
	if len(previews) != 1 {
		t.Fatalf("expected 1 preview, got %d", len(previews))
	}
	if previews[0].PreviewName != "Large" {
		t.Errorf("expected 'Large', got '%s'", previews[0].PreviewName)
	}
}

func TestScanFile_MultiplePreviews(t *testing.T) {
	dir := t.TempDir()
	kt := filepath.Join(dir, "Previews.kt")
	os.WriteFile(kt, []byte(`package com.example

@Preview(name = "Light")
@Composable
fun LightPreview() {
    Screen(dark = false)
}

@Preview(name = "Dark")
@Composable
fun DarkPreview() {
    Screen(dark = true)
}
`), 0644)

	previews := scanFile(kt, ":app")
	if len(previews) != 2 {
		t.Fatalf("expected 2 previews, got %d", len(previews))
	}
	if previews[0].PreviewName != "Light" {
		t.Errorf("expected 'Light', got '%s'", previews[0].PreviewName)
	}
	if previews[1].PreviewName != "Dark" {
		t.Errorf("expected 'Dark', got '%s'", previews[1].PreviewName)
	}
}

func TestScanFile_NoPackage(t *testing.T) {
	dir := t.TempDir()
	kt := filepath.Join(dir, "NoPackage.kt")
	os.WriteFile(kt, []byte(`@Preview
@Composable
fun SimplePreview() {
    Text("Hello")
}
`), 0644)

	previews := scanFile(kt, ":app")
	if len(previews) != 1 {
		t.Fatalf("expected 1 preview, got %d", len(previews))
	}
	if previews[0].FQN != "NoPackageKt.SimplePreview" {
		t.Errorf("expected 'NoPackageKt.SimplePreview', got '%s'", previews[0].FQN)
	}
}

func TestScan_FullProject(t *testing.T) {
	root := t.TempDir()

	// Create :app module
	appDir := filepath.Join(root, "app")
	os.MkdirAll(filepath.Join(appDir, "src", "main", "java", "com", "example"), 0755)
	os.WriteFile(filepath.Join(appDir, "build.gradle.kts"), []byte("// app build"), 0644)
	os.WriteFile(filepath.Join(appDir, "src", "main", "java", "com", "example", "Screen.kt"), []byte(`package com.example

@Preview
@Composable
fun ScreenPreview() {}
`), 0644)

	// Create :feature module
	featDir := filepath.Join(root, "feature")
	os.MkdirAll(filepath.Join(featDir, "src", "main", "java", "com", "feature"), 0755)
	os.WriteFile(filepath.Join(featDir, "build.gradle.kts"), []byte("// feature build"), 0644)
	os.WriteFile(filepath.Join(featDir, "src", "main", "java", "com", "feature", "Card.kt"), []byte(`package com.feature

@Preview(name = "Card")
@Composable
fun CardPreview() {}
`), 0644)

	// Root build.gradle
	os.WriteFile(filepath.Join(root, "build.gradle.kts"), []byte("// root build"), 0644)

	result := Scan(root)
	if len(result.Modules) != 3 { // root + app + feature
		t.Fatalf("expected 3 modules, got %d", len(result.Modules))
	}
	if len(result.AllPreviews) != 2 {
		t.Fatalf("expected 2 total previews, got %d", len(result.AllPreviews))
	}
}
