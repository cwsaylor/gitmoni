# GitMoni

A terminal user interface (TUI) for monitoring multiple local Git repositories with syntax-highlighted diffs and lazygit/github desktop integration.

This grew out of a need for monitoring multiple repositories for local changes to be committed and pushed before switching to my laptop.

![License](https://img.shields.io/badge/license-MIT-blue.svg)
![Go Version](https://img.shields.io/badge/go-1.25.1-blue.svg)

## Features

- **Multi-repository monitoring**: Track changes across multiple Git repositories from a single interface
- **Real-time status**: View repository status with visual indicators (✅ clean, 🔄 changes, ❌ errors)
- **Remote repository tracking**: Monitor if repositories need pulling from remote with ⬇️ indicator
- **Automatic remote fetching**: Fetches remote updates on startup and refresh
- **Animated spinners**: Shows per-repository animated spinners during fetch operations
- **Concurrent operations**: Fetches all repositories in parallel for faster updates
- **Three-pane tabbed interface**: Navigate between repositories, files, and diff view with Tab/Shift+Tab keys
- **Command-line repository management**: Add (`-a`), list (`-l`), and delete (`-d`) repositories from command line
- **Unified refresh**: Single `r` key refreshes both local status and fetches remote updates
- **Syntax highlighting**: Colored diff output with support for multiple file types
- **Configurable git client**: Supports lazygit or any other git client via configuration
- **Customizable icons**: Choose between emoji or Nerd Font glyphs for status indicators
- **Enhanced layout**: Responsive 70/30 split for repository and file lists
- **Configuration management**: Persistent repository list stored in `~/.gitmoni.json`

## Installation

### Prerequisites

- Go 1.25.1 or later
- Git
- [lazygit](https://github.com/jesseduffield/lazygit) (optional, for enhanced Git operations)

### Install via Homebrew

```bash
brew tap cwsaylor/tap
brew install gitmoni
```

### Build from source

```bash
git clone git@github.com:cwsaylor/gitmoni.git
cd gitmoni
go build
```
Copy the gitmoni binary to a local bin directory in your $PATH.

## Usage

### Starting GitMoni

```bash
# Start the TUI interface
gitmoni

# Add a repository from command line
gitmoni -a /path/to/repository

# List all configured repositories
gitmoni -l

# Remove a repository from configuration
gitmoni -d /path/to/repository
# Or
cd /path/to/repository
gitmoni -d  .
```

### Keyboard Shortcuts

- **`r`** - Refresh all repository statuses and fetch remote updates
- **`Tab`** - Switch forward between repository, file, and diff panes
- **`Shift+Tab`** - Switch backward between repository, file, and diff panes
- **`↑/↓` or `k/j`** - Navigate up/down in current pane or scroll diff view
- **`Enter`** - Launch configured git client (lazygit by default) for the selected repository
- **`q` or `Ctrl+C`** - Quit the application

## Configuration

GitMoni stores its configuration in `~/.gitmoni.json`, or in the current directory. The application will look for this file in:

1. Current directory (`./.gitmoni.json`)
2. Home directory (`~/.gitmoni.json`)

### Example Configuration

```json
{
  "repositories": [
    "/home/user/project1",
    "/home/user/project2",
    "/home/user/work/repo1"
  ],
  "enter_command_binary": "lazygit -p $REPO",
  "icon_style": "glyphs"
}
```

### Configuration Options

- **`repositories`**: Array of absolute paths to Git repositories to monitor
- **`enter_command_binary`**: Command template to run when pressing Enter on a repository (see Git Client Configuration below)
- **`icon_style`**: Display style for status indicators
  - `"emoji"` (default): Use emoji icons (❌ ✅ 🔄 ⬇️)
  - `"glyphs"`: Use Nerd Font glyphs (   )

**Note**: When using `"glyphs"`, you need a [Nerd Font](https://www.nerdfonts.com) installed in your terminal (e.g., Hack Nerd Font, FiraCode Nerd Font, etc.)

### Adding Repositories

You can add repositories in two ways:

**Command Line:**
```bash
gitmoni -a /path/to/repository
# or
cd /path/to/repository
gitmoni -a .
```

**Configuration File:**
Manually edit `.gitmoni.json` and add repository paths to the `repositories` array.

### Git Client Configuration

The `enter_command_binary` setting is a command template that runs when you press Enter on a repository. GitMoni replaces the `$REPO` placeholder with the selected repository path, then splits the command by spaces and executes it directly (no shell involved).

- Use `$REPO` where you want the selected repo path inserted.
- Because the command is split on spaces (no shell parsing), complex quoting and chaining won’t work. If you need to `cd` or use shell features, create a tiny wrapper script and call that with `$REPO`.

Examples:

- Recommended (lazygit):
  - `"enter_command_binary": "lazygit -p $REPO"`
  - This opens lazygit pointed at the selected repository.

- VS Code:
  - `"enter_command_binary": "code $REPO"`

- GitHub Desktop:
  - `"enter_command_binary": "github open $REPO"`

Default:

- If you don’t set this, the default is `"lazygit"` without arguments. It will launch lazygit in your current working directory, which may not be the selected repo. For best results, set it explicitly to `"lazygit -p $REPO"`.

## Git Status Indicators

### Emoji Icons (default)
- **✅** - Repository is clean (no changes)
- **🔄** - Repository has changes (number in parentheses shows change count, displayed in green)
- **❌** - Error accessing repository or not a Git repository
- **⬇️** - Repository needs to be pulled from remote (appears before repository path)

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
