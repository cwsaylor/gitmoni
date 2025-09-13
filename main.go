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
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)


type focusedPane int

const (
	focusRepo focusedPane = iota
	focusFile
	focusDiff
)

// fetchCompleteMsg is sent when remote fetching is complete
type fetchCompleteMsg struct{}

// repoFetchStartMsg is sent when a specific repo starts fetching
type repoFetchStartMsg struct {
	repo string
}

// repoFetchCompleteMsg is sent when a specific repo completes fetching
type repoFetchCompleteMsg struct {
	repo string
}

type model struct {
	config          *Config
	focused         focusedPane
	width           int
	height          int
	repoList        list.Model
	fileList        list.Model
	diffView        viewport.Model
	selectedRepo    int
	selectedFile    int
	gitStatuses     map[string]GitStatus
	currentDiff     string
	launchLazyGit   bool
	lazyGitRepo     string
	isFetching      bool
	spinner         spinner.Model
	fetchingRepos   map[string]bool // Track which repos are currently fetching
	repoSpinners    map[string]spinner.Model // Store spinners for each repo
}

// Icon represents the different icon types we use
type Icon struct {
	Error    string
	Success  string
	Changed  string
	Pull     string
}

// getIcons returns the appropriate icons based on the config setting
func getIcons(iconStyle string) Icon {
	if iconStyle == "glyphs" {
		// Nerd Font glyphs
		return Icon{
			Error:   "", // nf-fa-times_circle
			Success: "", // nf-fa-check_circle
			Changed: "", // nf-fa-refresh
			Pull:    "", // nf-fa-download
		}
	}
	// Default to emoji
	return Icon{
		Error:   "❌",
		Success: "✅",
		Changed: "🔄",
		Pull:    "⬇️",
	}
}

type repoItem struct {
	path       string
	status     GitStatus
	iconStyle  string
	isFetching bool
	spinner    spinner.Model
}

func (i repoItem) FilterValue() string { return i.path }
func (i repoItem) Title() string {
	icons := getIcons(i.iconStyle)
	pullIcon := ""
	if i.status.HasRemote && i.status.NeedsPull {
		pullIcon = icons.Pull + " "
	}

	title := ""
	if i.status.HasError {
		title = fmt.Sprintf("%s %s%s", icons.Error, pullIcon, i.path)
	} else if len(i.status.Files) == 0 {
		title = fmt.Sprintf("%s %s%s", icons.Success, pullIcon, i.path)
	} else {
		title = fmt.Sprintf("%s %s%s (%d)", icons.Changed, pullIcon, i.path, len(i.status.Files))
	}

	// Apply green color to repos with changes
	if len(i.status.Files) > 0 && !i.status.HasError {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render(title)
	}
	return title
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

	// Show spinner and "Updating" when fetching
	if i.isFetching {
		return fmt.Sprintf("%s • %s Updating", baseDesc, i.spinner.View())
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
	repoList.SetShowPagination(false)

	fileDelegate := list.NewDefaultDelegate()
	fileList := list.New([]list.Item{}, fileDelegate, 0, 0)
	fileList.Title = "Changed Files"
	fileList.SetShowStatusBar(false)
	fileList.SetShowPagination(false)

	diffView := viewport.New(0, 0)

	// Initialize spinner
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("63")) // Bright blue color

	m := model{
		config:        config,
		focused:       focusRepo,
		repoList:      repoList,
		fileList:      fileList,
		diffView:      diffView,
		gitStatuses:   make(map[string]GitStatus),
		spinner:       s,
		isFetching:    true, // Start in fetching state
		fetchingRepos: make(map[string]bool),
		repoSpinners:  make(map[string]spinner.Model),
	}

	if len(config.Repositories) > 0 {
		// Do initial status check without fetching
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

		// Get or create spinner for this repo
		s, exists := m.repoSpinners[repo]
		if !exists {
			s = spinner.New()
			s.Spinner = spinner.Dot
			s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
			m.repoSpinners[repo] = s
		}

		items = append(items, repoItem{
			path:       repo,
			status:     status,
			iconStyle:  m.config.IconStyle,
			isFetching: m.fetchingRepos[repo],
			spinner:    s,
		})
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

// fetchRemotesCmd returns a command that fetches all remotes in the background
func fetchRemotesCmd(repos []string) tea.Cmd {
	var cmds []tea.Cmd
	for _, repo := range repos {
		r := repo // Capture for closure
		cmds = append(cmds, func() tea.Msg {
			fetchRemoteUpdates(r)
			return repoFetchCompleteMsg{repo: r}
		})
	}

	return tea.Sequence(cmds...)
}

func (m model) Init() tea.Cmd {
	// Start spinner and fetch remotes in background
	if m.isFetching && len(m.config.Repositories) > 0 {
		var cmds []tea.Cmd
		// Mark all repos as fetching and start their spinners
		for _, repo := range m.config.Repositories {
			m.fetchingRepos[repo] = true
			// Initialize and start each repo's spinner
			if _, exists := m.repoSpinners[repo]; !exists {
				s := spinner.New()
				s.Spinner = spinner.Dot
				s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
				m.repoSpinners[repo] = s
			}
			if s, exists := m.repoSpinners[repo]; exists {
				cmds = append(cmds, s.Tick)
			}
		}
		// Add global spinner and fetch command
		cmds = append(cmds, m.spinner.Tick)
		cmds = append(cmds, fetchRemotesCmd(m.config.Repositories))
		return tea.Batch(cmds...)
	}
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    var cmd tea.Cmd
    var cmds []tea.Cmd

    switch msg := msg.(type) {
    case repoFetchCompleteMsg:
        // Mark repo as no longer fetching and update its status
        delete(m.fetchingRepos, msg.repo)
        // Update just this repo's status
        m.gitStatuses[msg.repo] = checkGitStatus(msg.repo)
        m.updateRepoList()
        // If this was the selected repo, update the file list
        if m.selectedRepo < len(m.config.Repositories) && m.config.Repositories[m.selectedRepo] == msg.repo {
            m.updateFileList()
            if len(m.fileList.Items()) > 0 {
                m.updateDiff()
            }
        }
        // Check if all repos are done fetching
        if len(m.fetchingRepos) == 0 {
            m.isFetching = false
        } else {
            // Continue spinner updates for remaining repos
            return m, m.spinner.Tick
        }
        return m, nil

    case spinner.TickMsg:
        // Update spinner if we're still fetching
        if m.isFetching || len(m.fetchingRepos) > 0 {
            var tickCmds []tea.Cmd
            m.spinner, cmd = m.spinner.Update(msg)
            if cmd != nil {
                tickCmds = append(tickCmds, cmd)
            }

            // Update all fetching repo spinners and collect their commands
            for repo := range m.fetchingRepos {
                if s, exists := m.repoSpinners[repo]; exists {
                    updatedSpinner, spinnerCmd := s.Update(msg)
                    m.repoSpinners[repo] = updatedSpinner
                    if spinnerCmd != nil {
                        tickCmds = append(tickCmds, spinnerCmd)
                    }
                }
            }

            // Update the repo list to show new spinner states
            m.updateRepoList()

            // Continue ticking all spinners
            return m, tea.Batch(tickCmds...)
        }

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
		rightColumnWidth := m.width - leftColumnWidth - 4 // Subtract 1 to prevent overflow

		// Help text takes up some vertical space
		helpHeight := 2 // Help text + some padding
		availableHeight := m.height - helpHeight

		// Left column is split vertically: repositories (70%) and files (30%)
		// Compute total content budget first to avoid rounding overflow, then split.
		leftPaneContentWidth := leftColumnWidth - frameWidth
		if leftPaneContentWidth < 0 {
			leftPaneContentWidth = 0
		}
		rightPaneContentWidth := rightColumnWidth - frameWidth
		if rightPaneContentWidth < 0 {
			rightPaneContentWidth = 0
		}

		leftContentBudget := availableHeight - (2 * frameHeight)
		if leftContentBudget < 0 {
			leftContentBudget = 0
		}
		repoHeight := (leftContentBudget * 7) / 10
		fileHeight := leftContentBudget - repoHeight

		diffHeight := availableHeight - frameHeight
		if diffHeight < 0 {
			diffHeight = 0
		}

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
    			// Switch focus between repo, file, and diff panes
    			if m.focused == focusRepo {
    				m.focused = focusFile
    			} else if m.focused == focusFile {
    				m.focused = focusDiff
    			} else {
    				m.focused = focusRepo
    			}
    		case "shift+tab":
    			// Switch focus backwards between repo, file, and diff panes
    			if m.focused == focusRepo {
    				m.focused = focusDiff
    			} else if m.focused == focusFile {
    				m.focused = focusRepo
    			} else {
    				m.focused = focusFile
    			}
    		case "up", "k":
    			// Route navigation to the focused pane only
    			if m.focused == focusRepo {
    				m.repoList, cmd = m.repoList.Update(msg)
    				cmds = append(cmds, cmd)
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
    				m.fileList, cmd = m.fileList.Update(msg)
    				cmds = append(cmds, cmd)
    				if m.fileList.SelectedItem() != nil {
    					m.selectedFile = m.fileList.Index()
    					m.updateDiff()
    				}
    				return m, tea.Batch(cmds...)
    			} else if m.focused == focusDiff {
    				m.diffView, cmd = m.diffView.Update(msg)
    				cmds = append(cmds, cmd)
    				return m, tea.Batch(cmds...)
    			}
    		case "down", "j":
    			// Route navigation to the focused pane only
    			if m.focused == focusRepo {
    				m.repoList, cmd = m.repoList.Update(msg)
    				cmds = append(cmds, cmd)
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
    				m.fileList, cmd = m.fileList.Update(msg)
    				cmds = append(cmds, cmd)
    				if m.fileList.SelectedItem() != nil {
    					m.selectedFile = m.fileList.Index()
    					m.updateDiff()
    				}
    				return m, tea.Batch(cmds...)
    			} else if m.focused == focusDiff {
    				m.diffView, cmd = m.diffView.Update(msg)
    				cmds = append(cmds, cmd)
    				return m, tea.Batch(cmds...)
    			}
    		case "r":
    			m.updateGitStatuses()
    			m.updateRepoList()
    			m.updateFileList()
    		case "f":
    			// Fetch remote updates for all repositories asynchronously
    			if !m.isFetching {
    				var fetchCmds []tea.Cmd
    				m.isFetching = true
    				// Mark all repos as fetching and start their spinners
    				for _, repo := range m.config.Repositories {
    					m.fetchingRepos[repo] = true
    					// Ensure spinner exists and start it
    					if _, exists := m.repoSpinners[repo]; !exists {
    						s := spinner.New()
    						s.Spinner = spinner.Dot
    						s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
    						m.repoSpinners[repo] = s
    					}
    					if s, exists := m.repoSpinners[repo]; exists {
    						fetchCmds = append(fetchCmds, s.Tick)
    					}
    				}
    				m.updateRepoList() // Update to show spinners
    				// Add global spinner and fetch command
    				fetchCmds = append(fetchCmds, m.spinner.Tick)
    				fetchCmds = append(fetchCmds, fetchRemotesCmd(m.config.Repositories))
    				return m, tea.Batch(fetchCmds...)
    			}
    		default:
    			// Forward all other key events (e.g. PgUp/PgDn) to the focused pane only
    			if m.focused == focusRepo {
    				m.repoList, cmd = m.repoList.Update(msg)
    				cmds = append(cmds, cmd)
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
    				m.fileList, cmd = m.fileList.Update(msg)
    				cmds = append(cmds, cmd)
    				if m.fileList.SelectedItem() != nil {
    					m.selectedFile = m.fileList.Index()
    					m.updateDiff()
    				}
    				return m, tea.Batch(cmds...)
    			} else if m.focused == focusDiff {
    				m.diffView, cmd = m.diffView.Update(msg)
    				cmds = append(cmds, cmd)
    				return m, tea.Batch(cmds...)
    			}
    		}
    	}

    // Only propagate non-key messages to other components to avoid duplicate key handling
    if _, isKey := msg.(tea.KeyMsg); !isKey {
        if m.focused != focusRepo {
            m.repoList, cmd = m.repoList.Update(msg)
            cmds = append(cmds, cmd)
        }
        if m.focused != focusFile {
            m.fileList, cmd = m.fileList.Update(msg)
            cmds = append(cmds, cmd)
        }
        if m.focused != focusDiff {
            m.diffView, cmd = m.diffView.Update(msg)
            cmds = append(cmds, cmd)
        }
    }


	return m, tea.Batch(cmds...)
}

func (m model) View() string {

	// Calculate left column width for proper pane sizing
	leftColumnWidth := int(float64(m.width) * 0.4)
	rightColumnWidth := m.width - leftColumnWidth - 4 // Subtract 1 to prevent overflow

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
	var repoPane, filePane, diffPane string
	if m.focused == focusRepo {
		repoPane = focusedStyle.Render(m.repoList.View())
		filePane = paneStyle.Render(m.fileList.View())
		diffPane = rightPaneStyle.Render(m.diffView.View())
	} else if m.focused == focusFile {
		repoPane = paneStyle.Render(m.repoList.View())
		filePane = focusedStyle.Render(m.fileList.View())
		diffPane = rightPaneStyle.Render(m.diffView.View())
	} else {
		repoPane = paneStyle.Render(m.repoList.View())
		filePane = paneStyle.Render(m.fileList.View())
		diffPane = rightPaneStyle.
			BorderForeground(lipgloss.Color("62")).
			Render(m.diffView.View())
	}

	// Create the left column by joining repo and file lists vertically
	leftColumn := lipgloss.JoinVertical(
		lipgloss.Left,
		repoPane,
		filePane,
	)

	// Create the right column with the diff view
	rightColumn := diffPane

	// Join the two columns horizontally
	content := lipgloss.JoinHorizontal(
		lipgloss.Top,
		leftColumn,
		rightColumn,
	)

    // Show spinner or help text
    var help string
    if m.isFetching {
        spinnerView := m.spinner.View()
        fetchText := lipgloss.NewStyle().
            Foreground(lipgloss.Color("240")).
            Render(" Fetching remote updates from repositories...")
        help = spinnerView + fetchText
    } else {
        helpText := fmt.Sprintf("Press 'r' to refresh, 'f' to fetch remotes, 'q' to quit, Tab to switch panes, ↑↓/PgUp/PgDn to navigate, Enter to open %s", m.config.EnterCommandBinary)
        help = lipgloss.NewStyle().
            Foreground(lipgloss.Color("240")).
            Width(m.width).
            Render(helpText)
    }

    joined := lipgloss.JoinVertical(lipgloss.Left, content, help)
    // Force the final frame to exactly match the terminal size to prevent scrollback growth
    return lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, joined)
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

    // Use the alternate screen to avoid polluting scrollback while the TUI runs.
    // If running inside tmux, ensure: set -g alternate-screen on
    p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}

	// Check if we need to launch the configured binary
	if result, ok := finalModel.(model); ok && result.launchLazyGit {
		commandTemplate := result.config.EnterCommandBinary

		// Replace $REPO with the selected repository path
		command := strings.ReplaceAll(commandTemplate, "$REPO", result.lazyGitRepo)

		// Split the command into program and arguments
		parts := strings.Fields(command)
		if len(parts) == 0 {
			return
		}

		// Execute the command
		var cmd *exec.Cmd
		if len(parts) == 1 {
			cmd = exec.Command(parts[0])
		} else {
			cmd = exec.Command(parts[0], parts[1:]...)
		}

		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Run()
	}
}
