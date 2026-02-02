package config

import (
	"encoding/json"
	"fmt"
	"net"
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
	ProjectName           string     `json:"project_name"`
	OdooVersion           string     `json:"odoo_version"`
	Branch                string     `json:"branch"`
	IsGitRepo             bool       `json:"is_git_repo"`
	ProjectRoot           string     `json:"project_root"`
	Modules               []string   `json:"modules"`
	Enterprise            bool       `json:"enterprise"`
	EnterpriseGitHubToken string     `json:"enterprise_github_token,omitempty"` // GitHub token for enterprise repo access
	WithoutDemo           bool       `json:"without_demo"`
	PipPackages           []string   `json:"pip_packages"`
	AddonsPaths           []string   `json:"addons_paths"`
	Ports                 Ports      `json:"ports"`
	CreatedAt             time.Time  `json:"created_at"`
	InitializedAt         *time.Time `json:"initialized_at,omitempty"` // When database was first initialized with -i
	BuiltAt               *time.Time `json:"built_at,omitempty"`       // When containers were first built with --build
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

// EnvironmentDir returns ~/.odooctl/{project}/{branch}
// This allows multiple environments per project (e.g., different branches or named environments)
func EnvironmentDir(projectName, branch string) (string, error) {
	projectDir, err := ProjectDir(projectName)
	if err != nil {
		return "", err
	}
	return filepath.Join(projectDir, branch), nil
}

// EnvironmentExists checks if an environment already exists
func EnvironmentExists(projectName, branch string) bool {
	dir, err := EnvironmentDir(projectName, branch)
	if err != nil {
		return false
	}

	statePath := filepath.Join(dir, StateFileName)
	_, err = os.Stat(statePath)
	return err == nil
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

// IsPortAvailable checks if a port is available on localhost
func IsPortAvailable(port int) bool {
	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	listener.Close()
	return true
}

// CheckPortsAvailable checks if all ports are available
func (p Ports) CheckPortsAvailable() (bool, []int) {
	var conflicting []int
	ports := []int{p.Odoo, p.Mailhog, p.SMTP, p.Debug}

	for _, port := range ports {
		if !IsPortAvailable(port) {
			conflicting = append(conflicting, port)
		}
	}

	return len(conflicting) == 0, conflicting
}

// FindAvailablePorts finds available ports starting from calculated ports
func FindAvailablePorts(version string) Ports {
	base := CalculatePorts(version)

	// Try to find available ports, incrementing by 10 if conflict
	for i := 0; i < 10; i++ {
		offset := i * 10
		candidate := Ports{
			Odoo:    base.Odoo + offset,
			Mailhog: base.Mailhog + offset,
			SMTP:    base.SMTP + offset,
			Debug:   base.Debug + offset,
		}

		available, _ := candidate.CheckPortsAvailable()
		if available {
			return candidate
		}
	}

	// Fall back to base if we couldn't find available ports
	return base
}

// Save writes state to the environment directory
func (s *State) Save() error {
	dir, err := EnvironmentDir(s.ProjectName, s.Branch)
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

// Load reads state from the environment directory
func Load(projectName, branch string) (*State, error) {
	dir, err := EnvironmentDir(projectName, branch)
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
// It searches through ~/.odooctl/{project}/{branch}/ directories
func LoadFromDir(dir string) (*State, error) {
	configDir, err := ConfigDir()
	if err != nil {
		return nil, err
	}

	// Iterate over project directories
	projectEntries, err := os.ReadDir(configDir)
	if err != nil {
		return nil, err
	}

	for _, projectEntry := range projectEntries {
		if !projectEntry.IsDir() {
			continue
		}

		projectDir := filepath.Join(configDir, projectEntry.Name())

		// Iterate over branch/environment directories within each project
		branchEntries, err := os.ReadDir(projectDir)
		if err != nil {
			continue
		}

		for _, branchEntry := range branchEntries {
			if !branchEntry.IsDir() {
				continue
			}

			state, err := Load(projectEntry.Name(), branchEntry.Name())
			if err != nil {
				continue
			}

			if state.ProjectRoot == dir {
				return state, nil
			}
		}
	}

	return nil, os.ErrNotExist
}
