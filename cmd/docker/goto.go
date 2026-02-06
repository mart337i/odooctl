package docker

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/egeskov/odooctl/internal/config"
	"github.com/egeskov/odooctl/pkg/prompt"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var gotoCmd = &cobra.Command{
	Use:   "goto",
	Short: "Switch to a different Docker project",
	Long: `Interactive project switcher that shows all available Odoo Docker projects
and allows you to switch between them.

The command will:
1. Show a tree view of all projects
2. Let you select a project
3. Change to that project's directory
4. Optionally checkout the associated git branch`,
	RunE: runGoto,
}

type projectInfo struct {
	Name        string
	Path        string
	Branch      string
	Version     string
	IsCurrent   bool
	ProjectRoot string
}

func runGoto(cmd *cobra.Command, args []string) error {
	cyan := color.New(color.FgCyan).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	dim := color.New(color.Faint).SprintFunc()

	configDir, err := config.ConfigDir()
	if err != nil {
		return err
	}

	// Get current directory to mark current project
	cwd, _ := os.Getwd()
	var currentProject string
	if state, err := config.LoadFromDir(cwd); err == nil {
		currentProject = state.ProjectName
	}

	// Scan for projects (new structure: ~/.odooctl/{project}/{branch}/)
	projectEntries, err := os.ReadDir(configDir)
	if err != nil {
		return fmt.Errorf("no projects found")
	}

	var projects []projectInfo

	for _, projectEntry := range projectEntries {
		if !projectEntry.IsDir() {
			continue
		}

		projectDir := filepath.Join(configDir, projectEntry.Name())
		branchEntries, err := os.ReadDir(projectDir)
		if err != nil {
			continue
		}

		for _, branchEntry := range branchEntries {
			if !branchEntry.IsDir() {
				continue
			}

			statePath := filepath.Join(projectDir, branchEntry.Name(), config.StateFileName)
			data, err := os.ReadFile(statePath)
			if err != nil {
				continue
			}

			var state config.State
			if err := json.Unmarshal(data, &state); err != nil {
				continue
			}

			projects = append(projects, projectInfo{
				Name:        state.ProjectName,
				Path:        filepath.Join(projectDir, branchEntry.Name()),
				Branch:      state.Branch,
				Version:     state.OdooVersion,
				IsCurrent:   state.ProjectName == currentProject && state.Branch == branchEntry.Name(),
				ProjectRoot: state.ProjectRoot,
			})
		}
	}

	if len(projects) == 0 {
		return fmt.Errorf("no valid projects found")
	}

	// Sort by name
	sort.Slice(projects, func(i, j int) bool {
		return projects[i].Name < projects[j].Name
	})

	// Display tree view
	fmt.Println("\nOdoo Docker Projects")
	fmt.Println("====================")

	for i, p := range projects {
		marker := "  "
		if p.IsCurrent {
			marker = yellow("→ ")
		}

		projectRoot := p.ProjectRoot
		if home, err := os.UserHomeDir(); err == nil {
			projectRoot = strings.Replace(projectRoot, home, "~", 1)
		}

		fmt.Printf("%s%d. %s/%s %s %s\n",
			marker,
			i+1,
			cyan(p.Name),
			cyan(p.Branch),
			dim(fmt.Sprintf("(Odoo %s)", p.Version)),
			dim(projectRoot),
		)
	}

	// Prompt for selection
	input, err := prompt.InputString(fmt.Sprintf("\nSelect project (1-%d) or 'q' to quit:", len(projects)), "")
	if err != nil || input == "q" || input == "Q" || input == "" {
		fmt.Println("Cancelled.")
		return nil
	}

	var selection int
	if _, err := fmt.Sscanf(input, "%d", &selection); err != nil || selection < 1 || selection > len(projects) {
		return fmt.Errorf("invalid selection")
	}

	selected := projects[selection-1]

	// Check if project root exists
	if _, err := os.Stat(selected.ProjectRoot); os.IsNotExist(err) {
		return fmt.Errorf("project path not found: %s", selected.ProjectRoot)
	}

	// Try git checkout if different branch
	if selected.Branch != "" {
		gitDir := filepath.Join(selected.ProjectRoot, ".git")
		if _, err := os.Stat(gitDir); err == nil {
			// Get current branch
			cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
			cmd.Dir = selected.ProjectRoot
			output, err := cmd.Output()
			if err == nil {
				currentBranch := strings.TrimSpace(string(output))
				if currentBranch != selected.Branch {
					// Check for uncommitted changes
					cmd := exec.Command("git", "status", "--porcelain")
					cmd.Dir = selected.ProjectRoot
					output, _ := cmd.Output()
					if len(output) > 0 {
						fmt.Printf("%s Uncommitted changes detected.\n", yellow("⚠️"))
						fmt.Printf("   Run: git stash && git checkout %s\n", selected.Branch)
					} else {
						// Checkout branch
						fmt.Printf("Checking out branch %s...\n", cyan(selected.Branch))
						cmd := exec.Command("git", "checkout", selected.Branch)
						cmd.Dir = selected.ProjectRoot
						cmd.Stdout = os.Stdout
						cmd.Stderr = os.Stderr
						_ = cmd.Run() // Best effort, errors will be visible in stderr
					}
				}
			}
		}
	}

	// Change to project directory and spawn new shell
	fmt.Printf("\nSwitching to %s (%s)\n", cyan(selected.Name), selected.ProjectRoot)
	fmt.Println("Starting new shell session...")

	if err := os.Chdir(selected.ProjectRoot); err != nil {
		return fmt.Errorf("failed to change directory: %w", err)
	}

	// Get shell
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}

	// Execute new shell
	shellCmd := exec.Command(shell)
	shellCmd.Stdin = os.Stdin
	shellCmd.Stdout = os.Stdout
	shellCmd.Stderr = os.Stderr
	shellCmd.Dir = selected.ProjectRoot

	return shellCmd.Run()
}
