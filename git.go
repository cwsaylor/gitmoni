package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type GitStatus struct {
	Path          string
	Files         []GitFile
	IsRepo        bool
	HasError      bool
	Error         string
	HasRemote     bool
	NeedsPull     bool
	RemoteStatus  string
}

type GitFile struct {
	Path   string
	Status string
}

func checkGitStatus(repoPath string) GitStatus {
	result := GitStatus{
		Path:   repoPath,
		Files:  []GitFile{},
		IsRepo: false,
	}

	if !isGitRepository(repoPath) {
		result.HasError = true
		result.Error = "Not a git repository"
		return result
	}

	result.IsRepo = true

	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		result.HasError = true
		result.Error = err.Error()
		return result
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		if len(line) >= 3 {
			status := strings.TrimSpace(line[:2])
			path := strings.TrimSpace(line[2:])

			// Remove quotes if git added them for paths with special characters
			if strings.HasPrefix(path, "\"") && strings.HasSuffix(path, "\"") {
				path = path[1 : len(path)-1]
			}

			result.Files = append(result.Files, GitFile{
				Path:   path,
				Status: status,
			})
		}
	}

	// Check remote status
	checkRemoteStatus(&result)

	return result
}

func isGitRepository(path string) bool {
	gitPath := filepath.Join(path, ".git")
	_, err := os.Stat(gitPath)
	return err == nil
}

func getFileDiff(repoPath, filePath string) (string, error) {
	// First try working directory changes
	cmd := exec.Command("git", "diff", "HEAD", "--", filePath)
	cmd.Dir = repoPath
	output, err := cmd.Output()

	// If no working directory changes, try staged changes
	if err != nil || len(output) == 0 {
		cmd = exec.Command("git", "diff", "--cached", "--", filePath)
		cmd.Dir = repoPath
		output, err = cmd.Output()

		// If no staged changes and file is untracked, show file content
		if err != nil || len(output) == 0 {
			cmd = exec.Command("git", "status", "--porcelain", "--", filePath)
			cmd.Dir = repoPath
			statusOutput, statusErr := cmd.Output()
			if statusErr == nil && strings.HasPrefix(strings.TrimSpace(string(statusOutput)), "??") {
				// File is untracked, show its content
				cmd = exec.Command("cat", filePath)
				cmd.Dir = repoPath
				content, contentErr := cmd.Output()
				if contentErr == nil {
					return fmt.Sprintf("New file: %s\n\n%s", filePath, string(content)), nil
				}
			}
		}
	}

	if err != nil {
		return "", err
	}
	return string(output), nil
}

func checkRemoteStatus(status *GitStatus) {
	// Check if there's a remote configured
	cmd := exec.Command("git", "remote")
	cmd.Dir = status.Path
	output, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(output)) == "" {
		status.HasRemote = false
		return
	}
	
	status.HasRemote = true

	// Get current branch
	cmd = exec.Command("git", "branch", "--show-current")
	cmd.Dir = status.Path
	branchOutput, err := cmd.Output()
	if err != nil {
		status.RemoteStatus = "Unable to get current branch"
		return
	}
	
	currentBranch := strings.TrimSpace(string(branchOutput))
	if currentBranch == "" {
		status.RemoteStatus = "No current branch"
		return
	}

	// Check if branch has upstream
	cmd = exec.Command("git", "rev-parse", "--abbrev-ref", currentBranch+"@{upstream}")
	cmd.Dir = status.Path
	upstreamOutput, err := cmd.Output()
	if err != nil {
		status.RemoteStatus = "No upstream branch"
		return
	}
	
	upstream := strings.TrimSpace(string(upstreamOutput))

	// Skip automatic fetch to avoid performance issues
	// Remote status will be based on last fetch time

	// Check if local is behind remote
	cmd = exec.Command("git", "rev-list", "--count", currentBranch+".."+upstream)
	cmd.Dir = status.Path
	behindOutput, err := cmd.Output()
	if err != nil {
		status.RemoteStatus = "Unable to check remote status"
		return
	}
	
	behindCount := strings.TrimSpace(string(behindOutput))
	if behindCount != "0" {
		status.NeedsPull = true
		status.RemoteStatus = fmt.Sprintf("%s commits behind", behindCount)
	} else {
		status.NeedsPull = false
		status.RemoteStatus = "Up to date"
	}
}

func fetchRemoteUpdates(repoPath string) error {
	cmd := exec.Command("git", "fetch", "--quiet")
	cmd.Dir = repoPath
	return cmd.Run()
}
