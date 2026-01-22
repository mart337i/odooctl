package docker

import (
	"github.com/egeskov/odooctl/internal/docker"
	"github.com/spf13/cobra"
)

var flagFollow bool

var logsCmd = &cobra.Command{
	Use:   "logs [service]",
	Short: "View container logs",
	Long:  `Shows logs from Docker containers. Defaults to the odoo service.`,
	RunE:  runLogs,
}

func init() {
	logsCmd.Flags().BoolVarP(&flagFollow, "follow", "f", false, "Follow log output")
}

func runLogs(cmd *cobra.Command, args []string) error {
	state, err := loadState()
	if err != nil {
		return err
	}

	service := "odoo"
	if len(args) > 0 {
		service = args[0]
	}

	logArgs := []string{"logs"}
	if flagFollow {
		logArgs = append(logArgs, "-f")
	}
	logArgs = append(logArgs, service)

	return docker.Compose(state, logArgs...)
}
