package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type GitStatus struct {
	Path     string
	Files    []GitFile
	IsRepo   bool
	HasError bool
	Error    string
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
