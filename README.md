# compose-preview

A terminal UI for browsing and running Jetpack Compose `@Preview` composables — without launching Android Studio.

![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)
![License](https://img.shields.io/badge/license-MIT-blue)
![Platforms](https://img.shields.io/badge/platforms-macOS%20%7C%20Linux%20%7C%20Windows-lightgrey)

## The problem

You want to check a Compose preview. You open Android Studio. You wait. And wait. Just to see a button in dark mode.

**compose-preview** scans your project for `@Preview` functions and lets you launch them on a connected device directly from the terminal.

## What it does

- **Scan** — Discovers all `@Preview` composables across all Gradle modules
- **Browse** — Navigate modules and previews in a two-panel TUI
- **Run** — Launch any preview on a connected device via ADB
- **Build** — Trigger Gradle builds without leaving the TUI
- **Filter** — Search previews by name
- **Details** — See FQN, file path, and annotation parameters

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

### Key bindings

| Key | Action |
|-----|--------|
| `j/k` or `↑/↓` | Navigate |
| `Tab` | Switch between Modules and Previews |
| `Enter` | Run selected preview on device |
| `b` | Build & install debug APK |
| `f` | Filter previews by name |
| `Esc` | Clear filter |
| `q` | Quit |

## Requirements

- **ADB** on your PATH (from Android SDK)
- A connected device or emulator
- The debug APK must be installed (use `b` to build+install)
- `androidx.compose.ui:ui-tooling` as a `debugImplementation` dependency

## How it works

1. Walks your Gradle project to discover modules (`build.gradle.kts`)
2. Scans `.kt` files for `@Preview` annotations
3. Extracts function names, packages, and JVM class names
4. Launches `PreviewActivity` via ADB with the composable FQN

Works with both pure Android and Kotlin Multiplatform (KMP) projects.

## License

MIT
