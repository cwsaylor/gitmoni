package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

type mode int

const (
	modeMain mode = iota
	modeFilePicker
)

type model struct {
	config           *Config
	mode             mode
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
		
		fileItem := items[index].(fileItem)
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
		
		paneWidth := m.width / 3
		paneHeight := m.height - 3
		
		m.repoList.SetSize(paneWidth-2, paneHeight)
		m.fileList.SetSize(paneWidth-2, paneHeight)
		m.diffView.Width = paneWidth - 2
		m.diffView.Height = paneHeight

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
			}
		}
	}

	switch m.mode {
	case modeMain:
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "up", "k":
				if m.repoList.SelectedItem() != nil {
					newIndex := (m.selectedRepo - 1 + len(m.config.Repositories)) % len(m.config.Repositories)
					m.selectRepo(newIndex)
				}
			case "down", "j":
				if m.repoList.SelectedItem() != nil {
					newIndex := (m.selectedRepo + 1) % len(m.config.Repositories)
					m.selectRepo(newIndex)
				}
			case "tab":
				items := m.fileList.Items()
				if len(items) > 0 {
					newIndex := (m.selectedFile + 1) % len(items)
					m.selectFile(newIndex)
				}
			case "shift+tab":
				items := m.fileList.Items()
				if len(items) > 0 {
					newIndex := (m.selectedFile - 1 + len(items)) % len(items)
					m.selectFile(newIndex)
				}
			case "r":
				m.updateGitStatuses()
				m.updateRepoList()
				m.updateFileList()
			}
		}
		
		m.repoList, cmd = m.repoList.Update(msg)
		cmds = append(cmds, cmd)
		
		m.fileList, cmd = m.fileList.Update(msg)
		cmds = append(cmds, cmd)
		
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

	leftPane := paneStyle.Render(m.repoList.View())
	middlePane := paneStyle.Render(m.fileList.View())
	rightPane := paneStyle.Render(m.diffView.View())

	content := lipgloss.JoinHorizontal(
		lipgloss.Top,
		leftPane,
		middlePane,
		rightPane,
	)

	helpText := "Press 'o' to add repository, 'r' to refresh, 'q' to quit, ↑↓ for repos, Tab/Shift+Tab for files"
	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Render(helpText)

	return lipgloss.JoinVertical(lipgloss.Left, content, help)
}

func main() {
	model, err := initialModel()
	if err != nil {
		fmt.Printf("Error initializing: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}
}