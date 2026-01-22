package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const StateFileName = ".odooctl-state.json"

type Ports struct {
	Odoo    int `json:"odoo"`
	Mailhog int `json:"mailhog"`
	SMTP    int `json:"smtp"`
	Debug   int `json:"debug"`
}

type State struct {
	ProjectName string    `json:"project_name"`
	OdooVersion string    `json:"odoo_version"`
	Branch      string    `json:"branch"`
	IsGitRepo   bool      `json:"is_git_repo"`
	ProjectRoot string    `json:"project_root"`
	Modules     []string  `json:"modules"`
	Enterprise  bool      `json:"enterprise"`
	WithoutDemo bool      `json:"without_demo"`
	PipPackages []string  `json:"pip_packages"`
	Ports       Ports     `json:"ports"`
	CreatedAt   time.Time `json:"created_at"`
}

// ConfigDir returns ~/.odooctl
func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".odooctl"), nil
}

// ProjectDir returns ~/.odooctl/{project}
func ProjectDir(projectName string) (string, error) {
	configDir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, projectName), nil
}

// CalculatePorts calculates ports based on Odoo version
func CalculatePorts(version string) Ports {
	// Parse major version (e.g., "17.0" -> 17)
	var major int
	if _, err := fmt.Sscanf(version, "%d", &major); err != nil {
		major = 17 // default
	}

	base := 8000 + (major * 100)
	return Ports{
		Odoo:    base,                      // e.g., 9700
		Mailhog: base + 25,                 // e.g., 9725
		SMTP:    1000 + (major * 100) + 25, // e.g., 1725
		Debug:   5000 + (major * 100) + 78, // e.g., 5778
	}
}

// Save writes state to the project directory
func (s *State) Save() error {
	dir, err := ProjectDir(s.ProjectName)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, StateFileName), data, 0644)
}

// Load reads state from the project directory
func Load(projectName string) (*State, error) {
	dir, err := ProjectDir(projectName)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(filepath.Join(dir, StateFileName))
	if err != nil {
		return nil, err
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	return &state, nil
}

// LoadFromDir tries to find state by looking for .odooctl-state.json in project dir
func LoadFromDir(dir string) (*State, error) {
	// First check if there's a state file that references this directory
	configDir, err := ConfigDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(configDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		state, err := Load(entry.Name())
		if err != nil {
			continue
		}

		if state.ProjectRoot == dir {
			return state, nil
		}
	}

	return nil, os.ErrNotExist
}
