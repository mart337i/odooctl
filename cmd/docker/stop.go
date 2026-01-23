package docker

import (
	"fmt"

	"github.com/egeskov/odooctl/internal/docker"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop all running containers",
	Long:  `Stop all Docker containers for this project without removing them.`,
	RunE:  runStop,
}

func runStop(cmd *cobra.Command, args []string) error {
	state, err := loadState()
	if err != nil {
		return err
	}

	green := color.New(color.FgGreen).SprintFunc()

	fmt.Println("Stopping containers...")
	if err := docker.Compose(state, "stop"); err != nil {
		return fmt.Errorf("failed to stop containers: %w", err)
	}

	fmt.Printf("\n%s Containers stopped!\n", green("✓"))
	return nil
}
