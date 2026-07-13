package docker

import (
	"fmt"

	"github.com/mart337i/odooctl/internal/docker"
	"github.com/mart337i/odooctl/internal/output"
	"github.com/spf13/cobra"
)

var flagStatusJSON bool

type statusReport struct {
	Project  string                `json:"project"`
	Version  string                `json:"version"`
	Database string                `json:"database"`
	Services []serviceStatusReport `json:"services"`
	URLs     map[string]string     `json:"urls,omitempty"`
}

type serviceStatusReport struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Status string `json:"status"`
	Ports  string `json:"ports"`
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show container status",
	Long:  `Displays the status of all Docker containers for this project.`,
	RunE:  runStatus,
}

func init() {
	statusCmd.Flags().BoolVar(&flagStatusJSON, "json", false, "Print JSON output")
}

func runStatus(cmd *cobra.Command, args []string) error {
	state, err := loadState()
	if err != nil {
		return err
	}
	if flagStatusJSON {
		services, err := docker.GetServicesStatus(state)
		if err != nil {
			return err
		}
		urls := make(map[string]string)
		serviceReports := make([]serviceStatusReport, 0, len(services))
		for _, svc := range services {
			serviceReports = append(serviceReports, serviceStatusReport{Name: svc.Name, State: svc.State, Status: svc.Status, Ports: svc.Ports})
			if svc.State == "running" && svc.Name == "odoo" {
				urls["odoo"] = fmt.Sprintf("http://localhost:%d", state.Ports.Odoo)
				urls["debug"] = fmt.Sprintf("localhost:%d", state.Ports.Debug)
			}
			if svc.State == "running" && svc.Name == "mailhog" {
				urls["mailhog"] = fmt.Sprintf("http://localhost:%d", state.Ports.Mailhog)
			}
		}
		return output.PrintJSON(statusReport{Project: state.ProjectName, Version: state.OdooVersion, Database: state.DBName(), Services: serviceReports, URLs: urls})
	}

	return docker.PrintStatus(state)
}
