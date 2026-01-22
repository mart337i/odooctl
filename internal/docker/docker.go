package docker

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/egeskov/odooctl/internal/config"
)

// Compose runs docker compose commands
func Compose(state *config.State, args ...string) error {
	dir, err := config.ProjectDir(state.ProjectName)
	if err != nil {
		return err
	}

	cmd := exec.Command("docker", append([]string{"compose"}, args...)...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}

// ComposeOutput runs docker compose and returns output
func ComposeOutput(state *config.State, args ...string) (string, error) {
	dir, err := config.ProjectDir(state.ProjectName)
	if err != nil {
		return "", err
	}

	cmd := exec.Command("docker", append([]string{"compose"}, args...)...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// IsRunning checks if containers are running
func IsRunning(state *config.State) bool {
	output, err := ComposeOutput(state, "ps", "--format", "{{.State}}")
	if err != nil {
		return false
	}
	return strings.Contains(output, "running")
}

// PrintStatus displays container status
func PrintStatus(state *config.State) error {
	fmt.Printf("Project: %s (Odoo %s)\n", state.ProjectName, state.OdooVersion)
	fmt.Printf("Directory: %s\n\n", state.ProjectRoot)
	return Compose(state, "ps")
}
