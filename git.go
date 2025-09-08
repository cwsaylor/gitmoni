package main

import (
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
	cmd := exec.Command("git", "diff", filePath)
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		cmd = exec.Command("git", "diff", "--cached", filePath)
		cmd.Dir = repoPath
		output, err = cmd.Output()
		if err != nil {
			return "", err
		}
	}
	return string(output), nil
}