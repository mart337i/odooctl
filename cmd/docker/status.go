package docker

import (
	"github.com/mart337i/odooctl/internal/docker"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show container status",
	Long:  `Displays the status of all Docker containers for this project.`,
	RunE:  runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
	state, err := loadState()
	if err != nil {
		return err
	}

	return docker.PrintStatus(state)
}
