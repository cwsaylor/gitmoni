package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type mode int

const (
	modeMain mode = iota
	modeFilePicker
)

type focusedPane int

const (
	focusRepo focusedPane = iota
	focusFile
)

type model struct {
	config           *Config
	mode             mode
	focused          focusedPane
	width            int
	height           int
	repoList         list.Model
	fileList         list.Model
	diffView         viewport.Model
	filePicker       filepicker.Model
	selectedRepo     int
	selectedFile     int
	gitStatuses      map[string]GitStatus
	currentDiff      string
	launchLazyGit    bool
	lazyGitRepo      string
}

type repoItem struct {
	path   string
	status GitStatus
}

func (i repoItem) FilterValue() string { return i.path }
func (i repoItem) Title() string       { 
	if i.status.HasError {
		return fmt.Sprintf("❌ %s", i.path)
	}
	if len(i.status.Files) == 0 {
		return fmt.Sprintf("✅ %s", i.path)
	}
	return fmt.Sprintf("🔄 %s (%d)", i.path, len(i.status.Files))
}
func (i repoItem) Description() string { 
	if i.status.HasError {
		return i.status.Error
	}
	if len(i.status.Files) == 0 {
		return "No changes"
	}
	return fmt.Sprintf("%d changed files", len(i.status.Files))
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

func initialModel() (model, error) {
	config, err := loadConfig()
	if err != nil {
		return model{}, err
	}

	fp := filepicker.New()
	fp.DirAllowed = true
	fp.FileAllowed = false

	repoDelegate := list.NewDefaultDelegate()
	repoList := list.New([]list.Item{}, repoDelegate, 0, 0)
	repoList.Title = "Repositories"

	fileDelegate := list.NewDefaultDelegate()
	fileList := list.New([]list.Item{}, fileDelegate, 0, 0)
	fileList.Title = "Changed Files"

	diffView := viewport.New(0, 0)

	m := model{
		config:      config,
		mode:        modeMain,
		focused:     focusRepo,
		repoList:    repoList,
		fileList:    fileList,
		diffView:    diffView,
		filePicker:  fp,
		gitStatuses: make(map[string]GitStatus),
	}

	if len(config.Repositories) == 0 {
		m.mode = modeFilePicker
	} else {
		m.updateGitStatuses()
		m.updateRepoList()
		if len(config.Repositories) > 0 {
			m.selectRepo(0)
		}
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
			m.currentDiff = diff
		}
		m.diffView.SetContent(m.currentDiff)
		m.diffView.GotoTop()
	}
}

func (m model) Init() tea.Cmd {
	if m.mode == modeFilePicker {
		return m.filePicker.Init()
	}
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
		
		// Available space inside frames
		leftContentWidth := leftColumnWidth - frameWidth
		rightContentWidth := rightColumnWidth - frameWidth
		
		// Help text takes up some vertical space
		helpHeight := 2 // Help text + some padding
		availableHeight := m.height - helpHeight
		
		// Left column is split vertically: repositories (40%) and files (60%)
		repoHeight := int(float64(availableHeight) * 0.4) - frameHeight
		fileHeight := availableHeight - repoHeight - frameHeight - frameHeight // Subtract frame overhead for both components
		
		// Right column gets remaining height
		diffHeight := availableHeight - frameHeight
		
		m.repoList.SetSize(leftContentWidth, repoHeight)
		m.fileList.SetSize(leftContentWidth, fileHeight)
		m.diffView.Width = rightContentWidth
		m.diffView.Height = diffHeight

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "o":
			if m.mode == modeMain {
				m.mode = modeFilePicker
				m.filePicker, cmd = m.filePicker.Update(msg)
				return m, cmd
			}
		case "esc":
			if m.mode == modeFilePicker {
				m.mode = modeMain
				return m, nil
			}
		case "enter":
			if m.mode == modeFilePicker {
				if didSelect, path := m.filePicker.DidSelectFile(msg); didSelect {
					m.config.addRepository(path)
					m.config.saveConfig()
					m.updateGitStatuses()
					m.updateRepoList()
					m.mode = modeMain
					if len(m.config.Repositories) > 0 {
						m.selectRepo(len(m.config.Repositories) - 1)
					}
				}
			} else if m.mode == modeMain && len(m.config.Repositories) > 0 {
				// Set flag to launch lazygit and quit
				m.launchLazyGit = true
				m.lazyGitRepo = m.config.Repositories[m.selectedRepo]
				return m, tea.Quit
			}
		}
	}

	switch m.mode {
	case modeMain:
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

	case modeFilePicker:
		m.filePicker, cmd = m.filePicker.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	if m.mode == modeFilePicker {
		return fmt.Sprintf("Select a directory to add as a repository:\n\n%s\n\nPress ESC to cancel", m.filePicker.View())
	}

	paneStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1)

	focusedStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(0, 1)

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
	rightColumn := paneStyle.Render(m.diffView.View())

	// Join the two columns horizontally
	content := lipgloss.JoinHorizontal(
		lipgloss.Top,
		leftColumn,
		rightColumn,
	)

	helpText := "Press 'o' to add repository, 'r' to refresh, 'q' to quit, Tab to switch panes, ↑↓ to navigate, Enter to open lazygit"
	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Render(helpText)

	return lipgloss.JoinVertical(lipgloss.Left, content, help)
}


func main() {
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

	// Check if we need to launch lazygit
	if result, ok := finalModel.(model); ok && result.launchLazyGit {
		cmd := exec.Command("lazygit", "-p", result.lazyGitRepo)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Run()
	}
}