# GitMoni

A terminal user interface (TUI) for monitoring multiple Git repositories with syntax-highlighted diffs and seamless lazygit integration.

![License](https://img.shields.io/badge/license-MIT-blue.svg)
![Go Version](https://img.shields.io/badge/go-1.25.1-blue.svg)

## Features

- **Multi-repository monitoring**: Track changes across multiple Git repositories from a single interface
- **Real-time status**: View repository status with visual indicators (✅ clean, 🔄 changes, ❌ errors)
- **Syntax highlighting**: Colored diff output with support for multiple file types
- **Lazygit integration**: Press Enter to launch lazygit for advanced Git operations
- **Two-pane layout**: Navigate between repositories and files with keyboard shortcuts
- **Configuration management**: Persistent repository list stored in `.gitmoni.json`

## Installation

### Prerequisites

- Go 1.25.1 or later
- Git
- [lazygit](https://github.com/jesseduffield/lazygit) (optional, for enhanced Git operations)

### Build from source

```bash
git clone git@github.com:cwsaylor/gitmoni.git
cd gitmoni
go build
```

## Usage

### Starting GitMoni

```bash
./gitmoni
```

### Keyboard Shortcuts

- **`o`** - Add a new repository (opens file picker)
- **`r`** - Refresh all repository statuses
- **`Tab`** - Switch between repository and file panes
- **`↑/↓` or `k/j`** - Navigate up/down in current pane
- **`Enter`** - Launch lazygit for the selected repository
- **`Esc`** - Cancel file picker mode
- **`q` or `Ctrl+C`** - Quit the application

### Interface Layout

```
┌─ Repositories (40%) ─┐┌─ Diff View (60%) ───┐
│ ✅ /path/to/repo1    ││ diff --git a/file   │
│ 🔄 /path/to/repo2 (3)││ @@ -1,3 +1,4 @@     │
│ ❌ /path/to/repo3    ││ +added line         │
├──────────────────────┤│ -removed line       │
│ Changed Files        ││                     │
│ M  src/main.go       ││                     │
│ A  README.md         ││                     │
│ ?? new_file.txt      ││                     │
└──────────────────────┘└─────────────────────┘
```

## Configuration

GitMoni stores its configuration in `.gitmoni.json`. The application will look for this file in:

1. Current directory (`./.gitmoni.json`)
2. Home directory (`~/.gitmoni.json`)

### Example Configuration

```json
{
  "repositories": [
    "/home/user/project1",
    "/home/user/project2",
    "/home/user/work/repo1"
  ]
}
```

### Adding Repositories

1. Press `o` to open the file picker
2. Navigate to the repository directory
3. Press `Enter` to add it to your monitored repositories
4. The configuration is automatically saved

## Git Status Indicators

- **✅** - Repository is clean (no changes)
- **🔄** - Repository has changes (number in parentheses shows change count)
- **❌** - Error accessing repository or not a Git repository

## File Status Codes

- **`M`** - Modified
- **`A`** - Added
- **`D`** - Deleted
- **`R`** - Renamed
- **`C`** - Copied
- **`U`** - Updated but unmerged
- **`??`** - Untracked

## Dependencies

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Bubbles](https://github.com/charmbracelet/bubbles) - TUI components
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) - Styling library
- [Chroma](https://github.com/alecthomas/chroma) - Syntax highlighting

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- Built with [Charm](https://charm.sh/) TUI libraries
- Inspired by the need for efficient multi-repository Git monitoring
