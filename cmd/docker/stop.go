package docker

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/mart337i/odooctl/internal/docker"
	"github.com/mart337i/odooctl/internal/output"
	"github.com/spf13/cobra"
)

var flagStopJSON bool

type stopReport struct {
	Stopped bool   `json:"stopped"`
	Output  string `json:"output,omitempty"`
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop all running containers",
	Long:  `Stop all Docker containers for this project without removing them.`,
	RunE:  runStop,
}

func init() {
	stopCmd.Flags().BoolVar(&flagStopJSON, "json", false, "Print JSON output")
}

func runStop(cmd *cobra.Command, args []string) error {
	state, err := loadState()
	if err != nil {
		return err
	}

	green := color.New(color.FgGreen).SprintFunc()
	if flagStopJSON {
		text, err := docker.ComposeOutput(state, "stop")
		if err != nil {
			return fmt.Errorf("failed to stop containers: %w", err)
		}
		return output.PrintJSON(stopReport{Stopped: true, Output: text})
	}

	fmt.Println("Stopping containers...")
	if err := docker.Compose(state, "stop"); err != nil {
		return fmt.Errorf("failed to stop containers: %w", err)
	}

	fmt.Printf("\n%s Containers stopped!\n", green("✓"))
	return nil
}
