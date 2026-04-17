# compose-preview

A terminal UI for browsing and running Jetpack Compose `@Preview` composables — without launching Android Studio.

![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)
![License](https://img.shields.io/badge/license-MIT-blue)
![Platforms](https://img.shields.io/badge/platforms-macOS%20%7C%20Linux%20%7C%20Windows-lightgrey)

## The problem

You want to check a Compose preview. You open Android Studio. You wait 2 minutes for it to index. You navigate to the file. You wait for the preview to render. All that just to see a button in dark mode.

**compose-preview** scans your project for `@Preview` functions and lets you browse and launch them on a connected device — directly from the terminal, in seconds.

## Screenshot

```
 Compose Preview Browser — umobKMP · Pixel_6a
 / Filter: login
╭ 1 Modules ──────────╮╭ 2 Previews (8) ──────────────────────╮╭ 3 Preview ─────────╮
│ ▸ :composeApp (0)    ││ ▸ LoginScreenEmptyDarkPreview        ││                     │
│   :feature:auth (8)  ││   LoginScreenEmptyLightPreview       ││   ┌───────────┐     │
│   :feature:booking   ││   LoginScreenFilledDarkPreview       ││   │           │     │
│     (0)              ││   LoginScreenLoadingDarkPreview      ││   │  preview  │     │
│   :feature:home (0)  ││   LoginScreenErrorDarkPreview        ││   │  image    │     │
│   :feature:map (0)   ││   LoginFormContentEmptyDarkPreview   ││   │           │     │
│   :feature:settings  ││   LoginFormContentFilledDarkPreview  ││   └───────────┘     │
│     (0)              ││   LoginFormContentLoadingDarkPreview ││                     │
│                      ││                                      ││ w for HD preview    │
│                      ││                                      ││   in browser        │
╰──────────────────────╯╰──────────────────────────────────────╯╰─────────────────────╯
╭ Details ────────────────────────────────────────────────────────────────────────────╮
│ FQN: com.example.feature.auth.LoginScreenPreviewKt.LoginScreenEmptyDarkPreview     │
│ File: feature/auth/src/androidMain/.../preview/LoginScreenPreview.kt:42            │
│ Params: showBackground=true, backgroundColor=0xFF111111                            │
╰─────────────────────────────────────────────────────────────────────────────────────╯
 ● Launched: LoginScreenEmptyDarkPreview (com.example.app.dev)
 enter run · s screenshot · w web · i install · / filter · d device · q quit
```

## What it does

- **Scan** — Discovers all `@Preview` composables across all Gradle modules automatically
- **Browse** — Navigate modules and previews in a three-panel TUI with keyboard and mouse
- **Search** — Live filter bar (`/`) matches preview names across all modules, counts update in real time
- **Run** — Launch any preview on a connected device via ADB with `Enter`
- **Screenshot** — Capture a preview screenshot (`s`) displayed directly in the terminal
- **HD Web Preview** — Press `w` to open a local web viewer in your browser with full-quality preview rendering
- **Install** — Trigger Gradle install tasks (`i`) with automatic variant detection (dev, qa, accept, production)
- **Device / Emulator picker** — Press `d` to select a connected device or launch an AVD emulator
- **Details** — See fully qualified name, file path, line number, and `@Preview` parameters
- **Stale detection** — Warns when source files are newer than the installed APK

## Install

### Homebrew (macOS & Linux)

```bash
brew tap ignaciotcrespo/tap
brew install compose-preview
```

### Go

```bash
go install github.com/ignaciotcrespo/compose-preview-cli/cmd/compose-preview@latest
```

### Binary download

Grab the latest release from [GitHub Releases](https://github.com/ignaciotcrespo/compose-preview-cli/releases).

## Usage

```bash
# From your Android/KMP project directory:
compose-preview

# Or specify a path:
compose-preview /path/to/android/project
```

### Layout

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ Compose Preview Browser — <project> · <device> (d to change)                │  header
├──────────────────────────────────────────────────────────────────────────────┤
│ / Press / to filter previews                                                │  search bar
├──────────────┬──────────────────────────┬────────────────────────────────────┤
│ 1 Modules    │ 2 Previews (88)          │ 3 Preview                         │
│              │                          │                                   │
│ ▸ :app (2)   │ ▸ AppAndroidPreview      │   ┌────────────┐                  │
│   :feature:  │   AppPreview             │   │  preview   │                  │
│     auth (30)│                          │   │  screenshot│  screenshot panel│
│   :feature:  │                          │   └────────────┘                  │
│     home (11)│                          │                                   │
│   ...        │                          │ w for HD preview in browser       │
├──────────────┴──────────────────────────┴────────────────────────────────────┤
│ Details                                                                      │
│ FQN: com.example.MainActivityKt.AppAndroidPreview                           │  details
│ File: composeApp/src/androidMain/.../MainActivity.kt:41                     │
├──────────────────────────────────────────────────────────────────────────────┤
│ ⚠ sources changed since last build — press 'i' to install                  │  status
├──────────────────────────────────────────────────────────────────────────────┤
│ enter run · s screenshot · w web · i install · / filter · d device · q quit │  help
└──────────────────────────────────────────────────────────────────────────────┘
```

### Key bindings

| Key | Action |
|-----|--------|
| `/` | Focus search bar — type to filter previews live |
| `Tab` | Exit search / switch between Modules and Previews panels |
| `Enter` | Run selected preview on device (auto-captures screenshot) |
| `Esc` | Clear filter and exit search |
| `j/k` or `↑/↓` | Navigate items in focused panel |
| `s` | Capture screenshot of the selected preview |
| `w` | Toggle HD web preview viewer in browser |
| `i` | Install APK via Gradle (auto-detects build variants) |
| `d` | Open device / emulator picker |
| `R` | Refresh project scan |
| `1` / `2` | Focus Modules / Previews panel directly |
| `q` | Quit |
| Mouse click | Select item in any panel |
| Mouse wheel | Scroll within panels |

### Search / Filter

Press `/` to activate the search bar. As you type, previews are filtered across **all modules** in real time:

```
 / Filter: dark█
╭ 1 Modules ─────────────╮╭ 2 Previews (23) ─────────────────────────────────╮
│ ▸ :feature:auth (12)    ││ ▸ LoginScreenEmptyDarkPreview (Login - Dark)     │
│   :feature:home (5)     ││   LoginScreenFilledDarkPreview (Login - Filled)  │
│   :feature:map (4)      ││   LoginScreenLoadingDarkPreview (Login - Loading)│
│   :feature:settings (2) ││   LoginScreenErrorDarkPreview (Login - Error)    │
│   :composeApp (0)       ││   ...                                            │
╰─────────────────────────╯╰──────────────────────────────────────────────────╯
 type to filter · tab panels · esc clear · enter confirm
```

Module counts update to show only matching previews. Press `Tab` to move to the panels with the filter active, or `Esc` to clear it.

### Screenshots

Press `s` to capture a screenshot of the selected preview. The screenshot is rendered directly in the terminal using half-block characters. Screenshots are cached — a dot marker (`◉`) next to a preview name indicates a cached screenshot.

Running a preview with `Enter` also auto-captures a screenshot after a short delay.

### HD Web Preview

The terminal screenshot is low resolution. For full-quality rendering, press `w` to start a local web viewer. This opens your browser with an HD version of the preview, served from a local web server. Press `w` again to stop the server.

### Device / Emulator picker

Press `d` to open a modal listing connected devices and available AVD emulators. Select a device to target, or pick an emulator to launch it.

### Install variants

When you press `i`, compose-preview queries Gradle for all available install tasks. If your project has multiple build variants (dev, qa, accept, production), a picker modal appears:

```
╭ Select Install Task ─────────────╮
│ ▸ installDevDebug                │
│   installAcceptDebug             │
│   installQaDebug                 │
│   installProductionDebug         │
│ ↑↓ navigate · enter select · esc │
╰──────────────────────────────────╯
```

Select a task with `Enter`. The choice is remembered for subsequent installs.

## Requirements

- **ADB** on your PATH (from Android SDK platform-tools)
- A connected Android device or emulator
- The debug APK must be installed on the device (use `b` to build+install)
- `androidx.compose.ui:ui-tooling` as a `debugImplementation` dependency in your app module:

```kotlin
// In your app's build.gradle.kts
debugImplementation("androidx.compose.ui:ui-tooling")
```

## How it works

1. **Discover** — Walks your Gradle project to find modules via `build.gradle.kts` files
2. **Scan** — Parses `.kt` files for `@Preview` annotations using regex (fast, no compilation needed)
3. **Resolve** — Extracts package name, JVM class name (`FileNameKt`), function name, and preview parameters
4. **Launch** — Sends `adb shell am start` with the composable FQN to `PreviewActivity`
5. **Detect** — Auto-discovers the installed app package, trying all flavor variants

Works with both pure Android and Kotlin Multiplatform (KMP) projects.

## Supported project structures

- Single-module Android projects
- Multi-module Android projects with feature modules
- Kotlin Multiplatform (KMP) projects with `composeApp` module
- Projects with product flavors (dev, qa, staging, production, etc.)
- Projects using `applicationId`, `namespace`, or both

## License

MIT
