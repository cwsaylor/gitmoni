package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	Repositories      []string `json:"repositories"`
	EnterCommandBinary string   `json:"enter_command_binary"`
	IconStyle         string   `json:"icon_style"`          // "emoji" or "glyphs"
	SortOrder         string   `json:"sort_order"`          // "manual" or "alphabetical"
	SortChangedToTop  bool     `json:"sort_changed_to_top"` // push changed/behind repos to top
}

func defaultConfig() *Config {
	return &Config{
		Repositories:       []string{},
		EnterCommandBinary: "lazygit", // default to lazygit
		IconStyle:          "emoji",   // default to emoji
		SortOrder:          "alphabetical", // default to alphabetical order
		SortChangedToTop:   true,           // default to floating changed repos to top
	}
}

func loadConfig() (*Config, error) {
	config := defaultConfig()

	configPaths := []string{
		".gitmoni.json",
		filepath.Join(os.Getenv("HOME"), ".gitmoni.json"),
	}

	for _, path := range configPaths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if err := json.Unmarshal(data, config); err != nil {
			continue
		}
		// Re-marshal the config with all fields (including new defaults)
		// and compare to what's on disk. If they differ, write back so
		// newly added fields appear in the file.
		updated, err := json.MarshalIndent(config, "", "  ")
		if err == nil && string(updated) != string(data) {
			os.WriteFile(path, updated, 0644)
		}
		return config, nil
	}

	// No config file found — write defaults to home directory
	homePath := filepath.Join(os.Getenv("HOME"), ".gitmoni.json")
	if data, err := json.MarshalIndent(config, "", "  "); err == nil {
		os.WriteFile(homePath, data, 0644)
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
