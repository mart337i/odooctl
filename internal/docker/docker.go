package docker

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/mart337i/odooctl/internal/config"
)

// CheckDaemon verifies that the Docker client can reach a running daemon.
func CheckDaemon() error {
	cmd := exec.Command("docker", "info", "--format", "{{.ServerVersion}}")
	output, err := cmd.CombinedOutput()
	return formatDaemonCheckError(strings.TrimSpace(string(output)), err)
}

func formatDaemonCheckError(output string, err error) error {
	if err == nil {
		return nil
	}
	if output == "" {
		output = err.Error()
	}
	return fmt.Errorf("Docker daemon is not available: %s\nStart Docker Desktop or the Docker service, then retry", output)
}

// CheckBindMount verifies that Docker can see files from a host directory.
func CheckBindMount(hostDir string) error {
	marker, err := os.CreateTemp(hostDir, ".odooctl-bind-check-*")
	if err != nil {
		return fmt.Errorf("failed to create bind-mount check file in %s: %w", hostDir, err)
	}
	markerName := filepath.Base(marker.Name())
	_ = marker.Close()
	defer os.Remove(marker.Name())

	cmd := exec.Command("docker", "run", "--rm", "-v", hostDir+":/mnt/odooctl-bind-check:ro", "alpine:latest", "test", "-f", "/mnt/odooctl-bind-check/"+markerName)
	output, err := cmd.CombinedOutput()
	return formatBindMountCheckError(hostDir, strings.TrimSpace(string(output)), err)
}

func formatBindMountCheckError(hostDir, output string, err error) error {
	if err == nil {
		return nil
	}
	if output != "" {
		output = ": " + output
	}
	return fmt.Errorf("Docker cannot access files under %s%s\nEnable Docker Desktop WSL integration for this distro or fix Docker file sharing, then retry", hostDir, output)
}

// Compose runs docker compose commands
func Compose(state *config.State, args ...string) error {
	dir, err := config.EnvironmentDir(state.ProjectName, state.Branch)
	if err != nil {
		return err
	}

	cmd := exec.Command("docker", append([]string{"compose"}, args...)...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	// Pass GITHUB_TOKEN as environment variable if using GitHub token for enterprise
	if state.Enterprise && state.EnterpriseGitHubToken != "" {
		cmd.Env = append(os.Environ(), fmt.Sprintf("GITHUB_TOKEN=%s", state.EnterpriseGitHubToken))
	}

	return cmd.Run()
}

// ComposeCommand creates an exec.Cmd for docker compose without running it
func ComposeCommand(state *config.State, args ...string) *exec.Cmd {
	cmd := exec.Command("docker", append([]string{"compose"}, args...)...)
	return cmd
}

// ComposeOutput runs docker compose and returns output
func ComposeOutput(state *config.State, args ...string) (string, error) {
	dir, err := config.EnvironmentDir(state.ProjectName, state.Branch)
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

// ServiceInfo represents docker compose service status
type ServiceInfo struct {
	Name   string `json:"Service"`
	State  string `json:"State"`
	Status string `json:"Status"`
	Ports  string `json:"Ports"`
}

// GetServicesStatus gets detailed status of all services
func GetServicesStatus(state *config.State) ([]ServiceInfo, error) {
	output, err := ComposeOutput(state, "ps", "--format", "json", "-a")
	if err != nil {
		return nil, err
	}

	var services []ServiceInfo
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		var svc ServiceInfo
		if err := json.Unmarshal([]byte(line), &svc); err != nil {
			continue
		}
		services = append(services, svc)
	}

	return services, nil
}

// PrintStatus displays container status with rich table output
func PrintStatus(state *config.State) error {
	cyan := color.New(color.FgCyan).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()
	dim := color.New(color.Faint).SprintFunc()

	fmt.Printf("\n%s %s\n", cyan("Project:"), state.ProjectName)
	fmt.Printf("%s Odoo %s\n", cyan("Version:"), state.OdooVersion)
	fmt.Printf("%s %s\n\n", cyan("Database:"), state.DBName())

	services, err := GetServicesStatus(state)
	if err != nil || len(services) == 0 {
		fmt.Printf("%s No containers found\n", color.YellowString("⚠️"))
		fmt.Printf("Run '%s' to start containers\n", cyan("odooctl docker run"))
		return nil
	}

	// Print table header
	fmt.Println("Docker Services Status")
	fmt.Println(strings.Repeat("─", 60))
	fmt.Printf("%-15s %-12s %-20s %s\n", "SERVICE", "STATE", "STATUS", "PORTS")
	fmt.Println(strings.Repeat("─", 60))

	runningServices := make(map[string]bool)
	for _, svc := range services {
		stateColor := red
		if svc.State == "running" {
			stateColor = green
			runningServices[svc.Name] = true
		}

		// Format ports
		ports := svc.Ports
		if ports == "" {
			ports = "-"
		}

		fmt.Printf("%-15s %-12s %-20s %s\n",
			cyan(svc.Name),
			stateColor(svc.State),
			dim(svc.Status),
			ports,
		)
	}
	fmt.Println(strings.Repeat("─", 60))

	// Print access URLs if running
	if len(runningServices) > 0 {
		fmt.Printf("\n%s\n", green("Access URLs:"))
		if runningServices["odoo"] {
			fmt.Printf("  %s Odoo:    http://localhost:%d\n", cyan("🌐"), state.Ports.Odoo)
			fmt.Printf("  %s Debug:   localhost:%d\n", cyan("🔧"), state.Ports.Debug)
		}
		if runningServices["mailhog"] {
			fmt.Printf("  %s MailHog: http://localhost:%d\n", cyan("📧"), state.Ports.Mailhog)
		}
	}

	return nil
}
