package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	Repositories      []string `json:"repositories"`
	EnterCommandBinary string   `json:"enter_command_binary"`
}

func loadConfig() (*Config, error) {
	config := &Config{
		Repositories:      []string{},
		EnterCommandBinary: "lazygit", // default to lazygit
	}

	configPaths := []string{
		".gitmoni.json",
		filepath.Join(os.Getenv("HOME"), ".gitmoni.json"),
	}

	for _, path := range configPaths {
		if data, err := os.ReadFile(path); err == nil {
			if err := json.Unmarshal(data, config); err == nil {
				return config, nil
			}
		}
	}

	return config, nil
}

func (c *Config) saveConfig() error {
	configPath := ".gitmoni.json"
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		configPath = filepath.Join(os.Getenv("HOME"), ".gitmoni.json")
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

func (c *Config) addRepository(path string) {
	for _, repo := range c.Repositories {
		if repo == path {
			return
		}
	}
	c.Repositories = append(c.Repositories, path)
}

func (c *Config) addRepositoryWithPath(path string) bool {
	// Convert path to absolute for comparison
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path // fallback to original path
	}

	// Check for duplicates using absolute paths
	for _, repo := range c.Repositories {
		existingAbs, err := filepath.Abs(repo)
		if err != nil {
			existingAbs = repo // fallback to original path
		}
		if existingAbs == absPath {
			return false // duplicate found
		}
	}
	
	c.Repositories = append(c.Repositories, absPath)
	return true // successfully added
}

func (c *Config) removeRepository(path string) bool {
	// Convert path to absolute for comparison
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path // fallback to original path
	}

	// Find and remove the repository using absolute paths
	for i, repo := range c.Repositories {
		existingAbs, err := filepath.Abs(repo)
		if err != nil {
			existingAbs = repo // fallback to original path
		}
		if existingAbs == absPath {
			// Remove the repository by creating a new slice without this element
			c.Repositories = append(c.Repositories[:i], c.Repositories[i+1:]...)
			return true // successfully removed
		}
	}
	
	return false // repository not found
}
