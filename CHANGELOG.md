# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.8.1] - 2026-03-19

### Fixed

- Fix repo selection (Enter, file list, diff) opening wrong repo when sorting is active

## [0.8.0] - 2026-03-19

### Added

- Show current branch name in each repo's description line
- Configurable repo sorting via `sort_order`: `"manual"` or `"alphabetical"` (default)
- Option to float changed/behind repos to top via `sort_changed_to_top` (default `true`), grouped by priority: both changed, remote only, local only, clean
- Highlight repos behind remote in yellow (Peach)
- Show repo directory name instead of full path by default (`display_full_path` option)
- Auto-backfill missing config fields to existing config files
- Write default config to `~/.gitmoni.json` on first run

### Changed

- Switch color scheme to Catppuccin Frappé (UI colors, list styles, and syntax highlighting)

## [0.7.0] - 2026-03-19

### Fixed

- Fix intermittent layout misalignment on startup caused by rendering before terminal dimensions are known
- Fix Init() value-receiver silently discarding fetching/spinner state
- Fix duplicate key-event dispatch that made pane navigation fragile
- Fix unchecked type assertion in diff view that could panic
- Fix selectedFile index diverging from list cursor after switching repos
- Fix git fetch errors being silently discarded with no UI feedback
- Replace shell `cat` with `os.ReadFile` for untracked file diffs (portability and path traversal safety)

### Removed

- Remove unused `addRepository` method from config

## [0.6.0] - 2025-10-04

### Changed

- GUI apps (e.g. GitHub Desktop) now launch in the background, keeping gitmoni running

## [0.5.0] - 2025-09-26

### Added

- Version flag (`-v` / `--version`)
- MIT License
- Homebrew install instructions in README

## [0.4.0] - 2025-09-13

### Added

- Fetch remote updates on app launch with a global progress spinner
- Per-repo spinners during remote fetch
- Concurrent remote fetching across all repositories
- Configurable icon style: emoji (default) or Nerd Font glyphs
- Shift+Tab to cycle panes backwards
- Color-highlighted repo names for repos with changes

### Fixed

- Fix right pane border rendering
- Fix app scrolling off-screen when navigating repos
- Fix pluralization of changed file counts

### Changed

- Simplify keyboard shortcuts to `r` for refresh (local + remote)

## [0.3.0] - 2025-09-10

### Added

- Remote repository status checking (ahead/behind tracking)
- Tab navigation to diff pane (three-pane focus cycle)

## [0.2.0] - 2025-09-09

### Added

- CLI repository management (`-a`, `-l`, `-d` flags)
- Configurable enter command binary via `enter_command_binary` in config
- Syntax highlighting for diffs using Chroma
- README

### Changed

- Left column split to 70/30 (repos/files)
- Switched to 2-column layout (left: repos + files, right: diff)

### Removed

- In-app repository adding (replaced by CLI flags)

## [0.1.0] - 2025-09-08

### Added

- Initial TUI for monitoring git repositories
- Three-pane interface: repositories, changed files, and diff view
- Press Enter to launch lazygit for the selected repo
- Git status monitoring with file change detection
- Support for paths with spaces
