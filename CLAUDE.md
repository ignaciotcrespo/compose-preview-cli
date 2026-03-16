# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test

```bash
./build.sh              # go build + go install
go test ./...           # run all tests
go test ./internal/scanner/ -v  # scanner tests
./test.sh --cover       # coverage report
./test.sh --cover --html  # open HTML coverage
```

## Architecture

compose-preview-cli is a Go TUI (Bubbletea) that scans Android/KMP projects for `@Preview` composables and lets you browse and run them on a device via ADB — without launching Android Studio. Entry point: `cmd/compose-preview/main.go` → finds Gradle project root → scans for previews → boots TUI.

**Three-layer design (same pattern as gitshelf):**

- **Controller** (`internal/controller/`) — Pure state machine, no I/O. `HandleKey()` takes immutable state + `KeyContext` snapshot, returns new state + actions.
- **Domain** (`internal/scanner/`, `internal/adb/`, `internal/gradle/`) — Kotlin file scanning, ADB device interaction, Gradle build integration.
- **UI** (`internal/ui/`) — Bubbletea Model implementing `Init`/`Update`/`View`. Split into `app.go` (model + update), `view.go` (layout), `render.go` (panel rendering), `styles.go` (colors).

**Shared types** (`internal/types/`) — Enums (PanelID, PanelState, PromptMode, ConfirmAction) live here to avoid circular imports.

**Reusable framework** — Uses `pkg/tui/` from gitshelf via Go module `replace` directive. Provides `Box`, `Prompt`, navigation helpers.

**Key concepts:**
- **Scanner**: Regex-based Kotlin parser that discovers `@Preview` annotations. Extracts package, function name, JVM class name (FileNameKt), preview params. FQN format: `package.FileNameKt.FunctionName`.
- **ADB launch**: Force-stops app, then starts `PreviewActivity` with composable FQN. Auto-detects installed flavor variants.
- **ApplicationId**: Detected at startup from app module (`:composeApp` or `:app`), handles both `applicationId` and `namespace`.

## Release

Uses goreleaser. To release:
```bash
git tag v0.1.0
git push --tags
```
CI runs tests, then goreleaser builds binaries for darwin/linux/windows (amd64/arm64) and publishes to GitHub Releases + Homebrew tap.
