package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/jroimartin/gocui"
)

// Version is set via ldflags at build time
var Version = "dev"

type focusedPane int

const (
	focusRepo focusedPane = iota
	focusFile
	focusDiff
)

type App struct {
	gui             *gocui.Gui
	config          *Config
	focused         focusedPane
	gitStatuses     map[string]GitStatus
	selectedRepo    int
	selectedFile    int
	currentDiff     string
	launchLazyGit   bool
	lazyGitRepo     string
	isFetching      bool
	fetchingRepos   map[string]bool
	mu              sync.Mutex
	spinnerFrame    int
	lastSpinnerTick time.Time
}

// Icon represents the different icon types we use
type Icon struct {
	Error   string
	Success string
	Changed string
	Pull    string
}

// getIcons returns the appropriate icons based on the config setting
func getIcons(iconStyle string) Icon {
	if iconStyle == "glyphs" {
		// Nerd Font glyphs
		return Icon{
			Error:   "", // nf-fa-times_circle
			Success: "", // nf-fa-check_circle
			Changed: "", // nf-fa-refresh
			Pull:    "", // nf-fa-download
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

func NewApp() (*App, error) {
	config, err := loadConfig()
	if err != nil {
		return nil, err
	}

	app := &App{
		config:        config,
		focused:       focusRepo,
		gitStatuses:   make(map[string]GitStatus),
		fetchingRepos: make(map[string]bool),
		isFetching:    true,
	}

	if len(config.Repositories) > 0 {
		// Do initial status check without fetching
		app.updateGitStatuses()
	}

	return app, nil
}

func (a *App) updateGitStatuses() {
	for _, repo := range a.config.Repositories {
		a.gitStatuses[repo] = checkGitStatus(repo)
	}
}

func (a *App) fetchRemotesAsync() {
	// Mark all repos as fetching
	a.mu.Lock()
	for _, repo := range a.config.Repositories {
		a.fetchingRepos[repo] = true
	}
	a.mu.Unlock()

	// Fetch all repos concurrently
	var wg sync.WaitGroup
	for _, repo := range a.config.Repositories {
		wg.Add(1)
		go func(r string) {
			defer wg.Done()
			fetchRemoteUpdates(r)

			// Update status for this repo
			a.mu.Lock()
			a.gitStatuses[r] = checkGitStatus(r)
			delete(a.fetchingRepos, r)
			a.mu.Unlock()
		}(repo)
	}

	// Wait for all to complete
	wg.Wait()

	a.mu.Lock()
	a.isFetching = false
	a.mu.Unlock()
}

func (a *App) layout(g *gocui.Gui) error {
	maxX, maxY := g.Size()

	// 2-column layout: left column (40%) for repo and file lists, right column (60%) for diff
	leftColumnWidth := int(float64(maxX) * 0.4)
	rightColumnStart := leftColumnWidth + 1

	// Help text takes up bottom 2 lines
	helpHeight := 2
	contentHeight := maxY - helpHeight

	// Left column is split vertically: repositories (70%) and files (30%)
	repoHeight := (contentHeight * 7) / 10
	fileStart := repoHeight + 1

	// Repository list view
	if v, err := g.SetView("repos", 0, 0, leftColumnWidth-1, repoHeight); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Title = "Repositories"
		v.Highlight = true
		v.SelBgColor = gocui.ColorGreen
		v.SelFgColor = gocui.ColorBlack
		a.updateRepoView(v)
	}

	// Files list view
	if v, err := g.SetView("files", 0, fileStart, leftColumnWidth-1, contentHeight-1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Title = "Changed Files"
		v.Highlight = true
		v.SelBgColor = gocui.ColorGreen
		v.SelFgColor = gocui.ColorBlack
		a.updateFileView(v)
	}

	// Diff view
	if v, err := g.SetView("diff", rightColumnStart, 0, maxX-1, contentHeight-1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Title = "Diff"
		v.Wrap = false
		v.Autoscroll = false
		a.updateDiffView(v)
	}

	// Help/status view
	if v, err := g.SetView("help", 0, contentHeight, maxX-1, maxY-1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Frame = false
		a.updateHelpView(v)
	}

	// Set initial focus
	if _, err := g.SetCurrentView("repos"); err != nil {
		return err
	}

	return nil
}

func (a *App) updateRepoView(v *gocui.View) {
	v.Clear()
	icons := getIcons(a.config.IconStyle)

	for i, repo := range a.config.Repositories {
		status := a.gitStatuses[repo]

		pullIcon := ""
		if status.HasRemote && status.NeedsPull {
			pullIcon = icons.Pull + " "
		}

		var line string
		isFetching := a.fetchingRepos[repo]

		if status.HasError {
			line = fmt.Sprintf("%s %s%s", icons.Error, pullIcon, filepath.Base(repo))
		} else if len(status.Files) == 0 {
			line = fmt.Sprintf("%s %s%s", icons.Success, pullIcon, filepath.Base(repo))
		} else {
			line = fmt.Sprintf("%s %s%s (%d)", icons.Changed, pullIcon, filepath.Base(repo), len(status.Files))
		}

		// Add fetching indicator
		if isFetching {
			spinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
			line += fmt.Sprintf(" %s Updating", spinner[a.spinnerFrame%len(spinner)])
		} else if status.HasRemote && status.RemoteStatus != "" {
			line += fmt.Sprintf(" • %s", status.RemoteStatus)
		}

		// Highlight selected item differently
		if i == a.selectedRepo {
			fmt.Fprintf(v, "> %s\n", line)
		} else {
			fmt.Fprintf(v, "  %s\n", line)
		}
	}
}

func (a *App) updateFileView(v *gocui.View) {
	v.Clear()

	if a.selectedRepo >= len(a.config.Repositories) {
		return
	}

	repo := a.config.Repositories[a.selectedRepo]
	status := a.gitStatuses[repo]

	if status.HasError {
		fmt.Fprintln(v, status.Error)
		return
	}

	if len(status.Files) == 0 {
		fmt.Fprintln(v, "No changes")
		return
	}

	for i, file := range status.Files {
		desc := getStatusDescription(file.Status)
		line := fmt.Sprintf("%s %s (%s)", file.Status, file.Path, desc)

		if i == a.selectedFile {
			fmt.Fprintf(v, "> %s\n", line)
		} else {
			fmt.Fprintf(v, "  %s\n", line)
		}
	}
}

func (a *App) updateDiffView(v *gocui.View) {
	v.Clear()

	if a.selectedRepo >= len(a.config.Repositories) {
		return
	}

	repo := a.config.Repositories[a.selectedRepo]
	status := a.gitStatuses[repo]

	if status.HasError || len(status.Files) == 0 {
		return
	}

	if a.selectedFile >= len(status.Files) {
		return
	}

	file := status.Files[a.selectedFile]
	diff, err := getFileDiff(repo, file.Path)

	if err != nil {
		fmt.Fprintf(v, "Error getting diff: %s", err.Error())
		return
	}

	if diff == "" {
		fmt.Fprintf(v, "No diff available for: %s\n\nThis could mean:\n- File is newly added (not tracked)\n- File is staged but no changes in working directory\n- Binary file", file.Path)
		return
	}

	// Apply syntax highlighting
	highlightedDiff := applySyntaxHighlighting(diff, file.Path)
	fmt.Fprint(v, highlightedDiff)
}

func (a *App) updateHelpView(v *gocui.View) {
	v.Clear()

	if a.isFetching {
		spinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		fmt.Fprintf(v, "%s Fetching remote updates from repositories...\n", spinner[a.spinnerFrame%len(spinner)])
	}

	fmt.Fprintf(v, "Press 'r' to refresh, 'q' to quit, Tab to switch panes, ↑↓ to navigate, Enter to open %s",
		a.config.EnterCommandBinary)
}

func (a *App) keybindings(g *gocui.Gui) error {
	// Quit
	if err := g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, a.quit); err != nil {
		return err
	}
	if err := g.SetKeybinding("", 'q', gocui.ModNone, a.quit); err != nil {
		return err
	}

	// Tab navigation between panes
	if err := g.SetKeybinding("", gocui.KeyTab, gocui.ModNone, a.nextView); err != nil {
		return err
	}

	// Refresh
	if err := g.SetKeybinding("", 'r', gocui.ModNone, a.refresh); err != nil {
		return err
	}

	// Enter to launch external command
	if err := g.SetKeybinding("repos", gocui.KeyEnter, gocui.ModNone, a.launchExternal); err != nil {
		return err
	}

	// Navigation for repos view
	if err := g.SetKeybinding("repos", gocui.KeyArrowUp, gocui.ModNone, a.cursorUp); err != nil {
		return err
	}
	if err := g.SetKeybinding("repos", 'k', gocui.ModNone, a.cursorUp); err != nil {
		return err
	}
	if err := g.SetKeybinding("repos", gocui.KeyArrowDown, gocui.ModNone, a.cursorDown); err != nil {
		return err
	}
	if err := g.SetKeybinding("repos", 'j', gocui.ModNone, a.cursorDown); err != nil {
		return err
	}

	// Navigation for files view
	if err := g.SetKeybinding("files", gocui.KeyArrowUp, gocui.ModNone, a.fileCursorUp); err != nil {
		return err
	}
	if err := g.SetKeybinding("files", 'k', gocui.ModNone, a.fileCursorUp); err != nil {
		return err
	}
	if err := g.SetKeybinding("files", gocui.KeyArrowDown, gocui.ModNone, a.fileCursorDown); err != nil {
		return err
	}
	if err := g.SetKeybinding("files", 'j', gocui.ModNone, a.fileCursorDown); err != nil {
		return err
	}

	// Navigation for diff view
	if err := g.SetKeybinding("diff", gocui.KeyArrowUp, gocui.ModNone, a.scrollUp); err != nil {
		return err
	}
	if err := g.SetKeybinding("diff", 'k', gocui.ModNone, a.scrollUp); err != nil {
		return err
	}
	if err := g.SetKeybinding("diff", gocui.KeyArrowDown, gocui.ModNone, a.scrollDown); err != nil {
		return err
	}
	if err := g.SetKeybinding("diff", 'j', gocui.ModNone, a.scrollDown); err != nil {
		return err
	}
	if err := g.SetKeybinding("diff", gocui.KeyPgup, gocui.ModNone, a.scrollPageUp); err != nil {
		return err
	}
	if err := g.SetKeybinding("diff", gocui.KeyPgdn, gocui.ModNone, a.scrollPageDown); err != nil {
		return err
	}

	return nil
}

func (a *App) quit(g *gocui.Gui, v *gocui.View) error {
	return gocui.ErrQuit
}

func (a *App) nextView(g *gocui.Gui, v *gocui.View) error {
	views := []string{"repos", "files", "diff"}
	current := g.CurrentView()

	if current == nil {
		return nil
	}

	currentName := current.Name()
	nextIndex := 0

	for i, name := range views {
		if name == currentName {
			nextIndex = (i + 1) % len(views)
			break
		}
	}

	_, err := g.SetCurrentView(views[nextIndex])

	// Update focused pane
	switch views[nextIndex] {
	case "repos":
		a.focused = focusRepo
	case "files":
		a.focused = focusFile
	case "diff":
		a.focused = focusDiff
	}

	return err
}

func (a *App) refresh(g *gocui.Gui, v *gocui.View) error {
	// Update local status immediately
	a.updateGitStatuses()

	// Refresh all views
	if repoView, err := g.View("repos"); err == nil {
		a.updateRepoView(repoView)
	}
	if fileView, err := g.View("files"); err == nil {
		a.updateFileView(fileView)
	}
	if diffView, err := g.View("diff"); err == nil {
		a.updateDiffView(diffView)
	}

	// Start async fetch if not already fetching
	if !a.isFetching {
		a.isFetching = true
		go a.fetchRemotesAsync()
	}

	return nil
}

func (a *App) launchExternal(g *gocui.Gui, v *gocui.View) error {
	if a.selectedRepo >= len(a.config.Repositories) {
		return nil
	}

	a.launchLazyGit = true
	a.lazyGitRepo = a.config.Repositories[a.selectedRepo]
	return gocui.ErrQuit
}

func (a *App) cursorUp(g *gocui.Gui, v *gocui.View) error {
	if a.selectedRepo > 0 {
		a.selectedRepo--
		a.selectedFile = 0

		// Update all views
		a.updateRepoView(v)
		if fileView, err := g.View("files"); err == nil {
			a.updateFileView(fileView)
		}
		if diffView, err := g.View("diff"); err == nil {
			a.updateDiffView(diffView)
		}
	}
	return nil
}

func (a *App) cursorDown(g *gocui.Gui, v *gocui.View) error {
	if a.selectedRepo < len(a.config.Repositories)-1 {
		a.selectedRepo++
		a.selectedFile = 0

		// Update all views
		a.updateRepoView(v)
		if fileView, err := g.View("files"); err == nil {
			a.updateFileView(fileView)
		}
		if diffView, err := g.View("diff"); err == nil {
			a.updateDiffView(diffView)
		}
	}
	return nil
}

func (a *App) fileCursorUp(g *gocui.Gui, v *gocui.View) error {
	if a.selectedRepo >= len(a.config.Repositories) {
		return nil
	}

	if a.selectedFile > 0 {
		a.selectedFile--

		// Update views
		a.updateFileView(v)
		if diffView, err := g.View("diff"); err == nil {
			a.updateDiffView(diffView)
		}
	}
	return nil
}

func (a *App) fileCursorDown(g *gocui.Gui, v *gocui.View) error {
	if a.selectedRepo >= len(a.config.Repositories) {
		return nil
	}

	repo := a.config.Repositories[a.selectedRepo]
	status := a.gitStatuses[repo]

	if a.selectedFile < len(status.Files)-1 {
		a.selectedFile++

		// Update views
		a.updateFileView(v)
		if diffView, err := g.View("diff"); err == nil {
			a.updateDiffView(diffView)
		}
	}
	return nil
}

func (a *App) scrollUp(g *gocui.Gui, v *gocui.View) error {
	ox, oy := v.Origin()
	if oy > 0 {
		return v.SetOrigin(ox, oy-1)
	}
	return nil
}

func (a *App) scrollDown(g *gocui.Gui, v *gocui.View) error {
	ox, oy := v.Origin()
	return v.SetOrigin(ox, oy+1)
}

func (a *App) scrollPageUp(g *gocui.Gui, v *gocui.View) error {
	ox, oy := v.Origin()
	_, height := v.Size()
	newY := oy - height
	if newY < 0 {
		newY = 0
	}
	return v.SetOrigin(ox, newY)
}

func (a *App) scrollPageDown(g *gocui.Gui, v *gocui.View) error {
	ox, oy := v.Origin()
	_, height := v.Size()
	return v.SetOrigin(ox, oy+height)
}

func (a *App) spinnerTick(g *gocui.Gui) {
	for {
		time.Sleep(100 * time.Millisecond)

		a.mu.Lock()
		if a.isFetching || len(a.fetchingRepos) > 0 {
			a.spinnerFrame++
			a.mu.Unlock()

			// Update views in UI thread
			g.Update(func(g *gocui.Gui) error {
				if repoView, err := g.View("repos"); err == nil {
					a.updateRepoView(repoView)
				}
				if helpView, err := g.View("help"); err == nil {
					a.updateHelpView(helpView)
				}
				return nil
			})
		} else {
			a.mu.Unlock()
		}
	}
}

func main() {
	// Parse command line flags
	addRepo := flag.String("a", "", "Add a repository to the config")
	listRepos := flag.Bool("l", false, "List repositories in the config")
	deleteRepo := flag.String("d", "", "Delete a repository from the config")
	versionShort := flag.Bool("v", false, "Display version")
	versionLong := flag.Bool("version", false, "Display version")
	flag.Parse()

	// Handle version flags
	if *versionShort || *versionLong {
		fmt.Println(Version)
		return
	}

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

	// Create app
	app, err := NewApp()
	if err != nil {
		fmt.Printf("Error initializing: %v\n", err)
		os.Exit(1)
	}

	// Initialize gocui
	g, err := gocui.NewGui(gocui.OutputNormal)
	if err != nil {
		log.Fatal(err)
	}
	defer g.Close()

	app.gui = g
	g.SetManagerFunc(app.layout)

	if err := app.keybindings(g); err != nil {
		log.Fatal(err)
	}

	// Start spinner animation
	go app.spinnerTick(g)

	// Start fetching remotes in background
	if len(app.config.Repositories) > 0 {
		go app.fetchRemotesAsync()
	}

	// Run the main loop
	if err := g.MainLoop(); err != nil && err != gocui.ErrQuit {
		log.Fatal(err)
	}

	// Check if we need to launch the external command
	if app.launchLazyGit {
		commandTemplate := app.config.EnterCommandBinary

		// Replace $REPO with the selected repository path
		command := strings.ReplaceAll(commandTemplate, "$REPO", app.lazyGitRepo)

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