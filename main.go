package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)


type focusedPane int

const (
	focusRepo focusedPane = iota
	focusFile
)

type model struct {
	config        *Config
	focused       focusedPane
	width         int
	height        int
	repoList      list.Model
	fileList      list.Model
	diffView      viewport.Model
	selectedRepo  int
	selectedFile  int
	gitStatuses   map[string]GitStatus
	currentDiff   string
	launchLazyGit bool
	lazyGitRepo   string
}

type repoItem struct {
	path   string
	status GitStatus
}

func (i repoItem) FilterValue() string { return i.path }
func (i repoItem) Title() string {
	pullIcon := ""
	if i.status.HasRemote && i.status.NeedsPull {
		pullIcon = "⬇️ "
	}
	
	if i.status.HasError {
		return fmt.Sprintf("❌ %s%s", pullIcon, i.path)
	}
	if len(i.status.Files) == 0 {
		return fmt.Sprintf("✅ %s%s", pullIcon, i.path)
	}
	return fmt.Sprintf("🔄 %s%s (%d)", pullIcon, i.path, len(i.status.Files))
}
func (i repoItem) Description() string {
	if i.status.HasError {
		return i.status.Error
	}
	
	baseDesc := ""
	if len(i.status.Files) == 0 {
		baseDesc = "No changes"
	} else {
		baseDesc = fmt.Sprintf("%d changed files", len(i.status.Files))
	}
	
	if i.status.HasRemote && i.status.RemoteStatus != "" {
		return fmt.Sprintf("%s • %s", baseDesc, i.status.RemoteStatus)
	}
	
	return baseDesc
}

type fileItem struct {
	gitFile GitFile
}

func (i fileItem) FilterValue() string { return i.gitFile.Path }
func (i fileItem) Title() string       { return fmt.Sprintf("%s %s", i.gitFile.Status, i.gitFile.Path) }
func (i fileItem) Description() string { return getStatusDescription(i.gitFile.Status) }

func getStatusDescription(status string) string {
	switch status {
	case "M":
		return "Modified"
	case "A":
		return "Added"
	case "D":
		return "Deleted"
	case "R":
		return "Renamed"
	case "C":
		return "Copied"
	case "U":
		return "Updated but unmerged"
	case "??":
		return "Untracked"
	default:
		return "Unknown"
	}
}

// applySyntaxHighlighting applies syntax highlighting to diff content
func applySyntaxHighlighting(content, filePath string) string {
	if content == "" {
		return content
	}

	// Check if this is a git diff format
	isDiff := strings.Contains(content, "diff --git") ||
		strings.Contains(content, "@@") ||
		strings.HasPrefix(content, "New file:")

	var lexer chroma.Lexer

	if isDiff {
		// Use diff lexer for git diff output
		lexer = lexers.Get("diff")
	} else {
		// For new files, try to detect lexer by file extension
		lexer = lexers.Match(filePath)
	}

	// Fallback to plain text if no lexer found
	if lexer == nil {
		lexer = lexers.Fallback
	}

	// Use a terminal-friendly style
	style := styles.Get("github-dark")
	if style == nil {
		style = styles.Fallback
	}

	// Create a 16-color terminal formatter for better compatibility
	formatter := formatters.Get("terminal16m")
	if formatter == nil {
		formatter = formatters.Fallback
	}

	// Apply syntax highlighting
	var buf strings.Builder
	iterator, err := lexer.Tokenise(nil, content)
	if err != nil {
		return content // Return original content if highlighting fails
	}

	err = formatter.Format(&buf, style, iterator)
	if err != nil {
		return content // Return original content if formatting fails
	}

	return buf.String()
}

func addRepositoryFromCommandLine(path string) error {
	// Load config
	config, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Expand path to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	// Check if directory exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return fmt.Errorf("directory does not exist: %s", absPath)
	}

	// Check if it's a git repository
	gitDir := filepath.Join(absPath, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return fmt.Errorf("not a git repository: %s", absPath)
	}

	// Add repository with duplicate checking
	if config.addRepositoryWithPath(absPath) {
		// Save config
		if err := config.saveConfig(); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
		fmt.Printf("Added repository: %s\n", absPath)
	} else {
		fmt.Printf("Repository already exists: %s\n", absPath)
	}

	return nil
}

func listRepositoriesFromCommandLine() error {
	// Load config
	config, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if len(config.Repositories) == 0 {
		fmt.Println("No repositories configured")
		return nil
	}

	fmt.Printf("Configured repositories (%d):\n", len(config.Repositories))
	for i, repo := range config.Repositories {
		fmt.Printf("%d. %s\n", i+1, repo)
	}

	return nil
}

func deleteRepositoryFromCommandLine(path string) error {
	// Load config
	config, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Expand path to absolute path for comparison
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	// Remove repository
	if config.removeRepository(absPath) {
		// Save config
		if err := config.saveConfig(); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
		fmt.Printf("Removed repository: %s\n", absPath)
	} else {
		fmt.Printf("Repository not found: %s\n", absPath)
	}

	return nil
}

func initialModel() (model, error) {
	config, err := loadConfig()
	if err != nil {
		return model{}, err
	}


	repoDelegate := list.NewDefaultDelegate()
	repoList := list.New([]list.Item{}, repoDelegate, 0, 0)
	repoList.Title = "Repositories"
	repoList.SetShowStatusBar(false)

	fileDelegate := list.NewDefaultDelegate()
	fileList := list.New([]list.Item{}, fileDelegate, 0, 0)
	fileList.Title = "Changed Files"
	fileList.SetShowStatusBar(false)

	diffView := viewport.New(0, 0)

	m := model{
		config:      config,
		focused:     focusRepo,
		repoList:    repoList,
		fileList:    fileList,
		diffView:    diffView,
		gitStatuses: make(map[string]GitStatus),
	}

	if len(config.Repositories) > 0 {
		m.updateGitStatuses()
		m.updateRepoList()
		m.selectRepo(0)
	}

	return m, nil
}

func (m *model) updateGitStatuses() {
	for _, repo := range m.config.Repositories {
		m.gitStatuses[repo] = checkGitStatus(repo)
	}
}

func (m *model) updateRepoList() {
	items := make([]list.Item, 0)
	for _, repo := range m.config.Repositories {
		status, exists := m.gitStatuses[repo]
		if !exists {
			status = GitStatus{Path: repo, HasError: true, Error: "Status not loaded"}
		}
		items = append(items, repoItem{path: repo, status: status})
	}
	m.repoList.SetItems(items)
}

func (m *model) updateFileList() {
	if m.selectedRepo >= len(m.config.Repositories) {
		return
	}

	repo := m.config.Repositories[m.selectedRepo]
	status, exists := m.gitStatuses[repo]
	if !exists || status.HasError {
		m.fileList.SetItems([]list.Item{})
		return
	}

	items := make([]list.Item, 0)
	for _, file := range status.Files {
		items = append(items, fileItem{gitFile: file})
	}
	m.fileList.SetItems(items)
}

func (m *model) selectRepo(index int) {
	if index >= 0 && index < len(m.config.Repositories) {
		m.selectedRepo = index
		m.repoList.Select(index)
		m.updateFileList()
		if len(m.fileList.Items()) > 0 {
			m.selectFile(0)
		} else {
			m.currentDiff = ""
			m.diffView.SetContent("")
		}
	}
}

func (m *model) selectFile(index int) {
	items := m.fileList.Items()
	if index >= 0 && index < len(items) {
		m.selectedFile = index
		m.fileList.Select(index)
		m.updateDiff()
	}
}

func (m *model) updateDiff() {
	items := m.fileList.Items()
	if m.selectedFile >= 0 && m.selectedFile < len(items) {
		fileItem := items[m.selectedFile].(fileItem)
		repo := m.config.Repositories[m.selectedRepo]

		diff, err := getFileDiff(repo, fileItem.gitFile.Path)
		if err != nil {
			m.currentDiff = fmt.Sprintf("Error getting diff: %s", err.Error())
		} else if diff == "" {
			m.currentDiff = fmt.Sprintf("No diff available for: %s\n\nThis could mean:\n- File is newly added (not tracked)\n- File is staged but no changes in working directory\n- Binary file", fileItem.gitFile.Path)
		} else {
			// Apply syntax highlighting to the diff content
			highlightedDiff := applySyntaxHighlighting(diff, fileItem.gitFile.Path)
			m.currentDiff = highlightedDiff
		}
		m.diffView.SetContent(m.currentDiff)
		m.diffView.GotoTop()
	}
}

func (m *model) fetchAllRemotes() {
	for _, repo := range m.config.Repositories {
		fetchRemoteUpdates(repo) // Don't block on errors
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Create a style to calculate frame size including borders and padding
		frameStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(0, 1)

		// Calculate frame overhead (borders + padding)
		frameWidth, frameHeight := frameStyle.GetFrameSize()

		// 2-column layout: left column (40%) for repo and file lists, right column (60%) for diff
		leftColumnWidth := int(float64(m.width) * 0.4)
		rightColumnWidth := m.width - leftColumnWidth

		// Help text takes up some vertical space
		helpHeight := 2 // Help text + some padding
		availableHeight := m.height - helpHeight

		// Left column is split vertically: repositories (70%) and files (30%)
		// Each component gets the full left column width and will have its own borders
		leftPaneContentWidth := leftColumnWidth - frameWidth
		rightPaneContentWidth := rightColumnWidth - frameWidth

		repoHeight := int(float64(availableHeight)*0.7) - frameHeight
		fileHeight := availableHeight - repoHeight - frameHeight - frameHeight // Subtract frame overhead for both components
		diffHeight := availableHeight - frameHeight

		m.repoList.SetSize(leftPaneContentWidth, repoHeight)
		m.fileList.SetSize(leftPaneContentWidth, fileHeight)
		m.diffView.Width = rightPaneContentWidth
		m.diffView.Height = diffHeight

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "enter":
			if len(m.config.Repositories) > 0 {
				// Set flag to launch lazygit and quit
				m.launchLazyGit = true
				m.lazyGitRepo = m.config.Repositories[m.selectedRepo]
				return m, tea.Quit
			}
		}
	}

		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "tab":
				// Switch focus between repo and file panes
				if m.focused == focusRepo {
					m.focused = focusFile
				} else {
					m.focused = focusRepo
				}
			case "up", "k":
				if m.focused == focusRepo {
					// Let the list handle the navigation, then sync our state
					m.repoList, cmd = m.repoList.Update(msg)
					cmds = append(cmds, cmd)
					// Update our selection tracking and file list
					if m.repoList.SelectedItem() != nil {
						m.selectedRepo = m.repoList.Index()
						m.updateFileList()
						if len(m.fileList.Items()) > 0 {
							m.selectFile(0)
						} else {
							m.currentDiff = ""
							m.diffView.SetContent("")
						}
					}
					return m, tea.Batch(cmds...)
				} else if m.focused == focusFile {
					// Let the list handle the navigation, then sync our state
					m.fileList, cmd = m.fileList.Update(msg)
					cmds = append(cmds, cmd)
					// Update our selection tracking and diff
					if m.fileList.SelectedItem() != nil {
						m.selectedFile = m.fileList.Index()
						m.updateDiff()
					}
					return m, tea.Batch(cmds...)
				}
			case "down", "j":
				if m.focused == focusRepo {
					// Let the list handle the navigation, then sync our state
					m.repoList, cmd = m.repoList.Update(msg)
					cmds = append(cmds, cmd)
					// Update our selection tracking and file list
					if m.repoList.SelectedItem() != nil {
						m.selectedRepo = m.repoList.Index()
						m.updateFileList()
						if len(m.fileList.Items()) > 0 {
							m.selectFile(0)
						} else {
							m.currentDiff = ""
							m.diffView.SetContent("")
						}
					}
					return m, tea.Batch(cmds...)
				} else if m.focused == focusFile {
					// Let the list handle the navigation, then sync our state
					m.fileList, cmd = m.fileList.Update(msg)
					cmds = append(cmds, cmd)
					// Update our selection tracking and diff
					if m.fileList.SelectedItem() != nil {
						m.selectedFile = m.fileList.Index()
						m.updateDiff()
					}
					return m, tea.Batch(cmds...)
				}
			case "r":
				m.updateGitStatuses()
				m.updateRepoList()
				m.updateFileList()
			case "f":
				// Fetch remote updates for all repositories
				go m.fetchAllRemotes()
				m.updateGitStatuses()
				m.updateRepoList()
				m.updateFileList()
			}
		}

	// Only update non-focused components
	if m.focused != focusRepo {
		m.repoList, cmd = m.repoList.Update(msg)
		cmds = append(cmds, cmd)
	}

	if m.focused != focusFile {
		m.fileList, cmd = m.fileList.Update(msg)
		cmds = append(cmds, cmd)
	}

	m.diffView, cmd = m.diffView.Update(msg)
	cmds = append(cmds, cmd)


	return m, tea.Batch(cmds...)
}

func (m model) View() string {

	// Calculate left column width for proper pane sizing
	leftColumnWidth := int(float64(m.width) * 0.4)
	rightColumnWidth := m.width - leftColumnWidth

	paneStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		Width(leftColumnWidth)

	focusedStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(0, 1).
		Width(leftColumnWidth)

	rightPaneStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		Width(rightColumnWidth)

	// Apply focused styling to the current pane
	var repoPane, filePane string
	if m.focused == focusRepo {
		repoPane = focusedStyle.Render(m.repoList.View())
		filePane = paneStyle.Render(m.fileList.View())
	} else {
		repoPane = paneStyle.Render(m.repoList.View())
		filePane = focusedStyle.Render(m.fileList.View())
	}

	// Create the left column by joining repo and file lists vertically
	leftColumn := lipgloss.JoinVertical(
		lipgloss.Left,
		repoPane,
		filePane,
	)

	// Create the right column with the diff view
	rightColumn := rightPaneStyle.Render(m.diffView.View())

	// Join the two columns horizontally
	content := lipgloss.JoinHorizontal(
		lipgloss.Top,
		leftColumn,
		rightColumn,
	)

	helpText := fmt.Sprintf("Press 'r' to refresh, 'f' to fetch remotes, 'q' to quit, Tab to switch panes, ↑↓ to navigate, Enter to open %s", m.config.EnterCommandBinary)
	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Render(helpText)

	return lipgloss.JoinVertical(lipgloss.Left, content, help)
}

func main() {
	// Parse command line flags
	addRepo := flag.String("a", "", "Add a repository to the config")
	listRepos := flag.Bool("l", false, "List repositories in the config")
	deleteRepo := flag.String("d", "", "Delete a repository from the config")
	flag.Parse()

	// Handle add repository command
	if *addRepo != "" {
		err := addRepositoryFromCommandLine(*addRepo)
		if err != nil {
			fmt.Printf("Error adding repository: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Handle list repositories command
	if *listRepos {
		err := listRepositoriesFromCommandLine()
		if err != nil {
			fmt.Printf("Error listing repositories: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Handle delete repository command
	if *deleteRepo != "" {
		err := deleteRepositoryFromCommandLine(*deleteRepo)
		if err != nil {
			fmt.Printf("Error deleting repository: %v\n", err)
			os.Exit(1)
		}
		return
	}

	m, err := initialModel()
	if err != nil {
		fmt.Printf("Error initializing: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}

	// Check if we need to launch the configured binary
	if result, ok := finalModel.(model); ok && result.launchLazyGit {
		binary := result.config.EnterCommandBinary
		
		// Handle different binaries with their specific arguments
		var cmd *exec.Cmd
		if binary == "lazygit" {
			// lazygit supports -p flag for specifying repo path
			cmd = exec.Command(binary, "-p", result.lazyGitRepo)
		} else {
			// For other binaries, change directory and run without arguments
			cmd = exec.Command(binary)
			cmd.Dir = result.lazyGitRepo
		}
		
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Run()
	}
}
