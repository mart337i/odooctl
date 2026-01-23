package docker

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/egeskov/odooctl/internal/config"
	"github.com/fatih/color"
)

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

	return cmd.Run()
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
	fmt.Printf("%s %s\n\n", cyan("Database:"), getDBNameFromState(state))

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

	hasRunning := false
	for _, svc := range services {
		stateColor := red
		if svc.State == "running" {
			stateColor = green
			hasRunning = true
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
	if hasRunning {
		fmt.Printf("\n%s\n", green("Access URLs:"))
		fmt.Printf("  %s Odoo:    http://localhost:%d\n", cyan("🌐"), state.Ports.Odoo)
		fmt.Printf("  %s MailHog: http://localhost:%d\n", cyan("📧"), state.Ports.Mailhog)
		fmt.Printf("  %s Debug:   localhost:%d\n", cyan("🔧"), state.Ports.Debug)
	}

	return nil
}

func getDBNameFromState(state *config.State) string {
	versionSuffix := strings.Replace(state.OdooVersion, ".", "", 1)
	return "odoo-" + versionSuffix
}
