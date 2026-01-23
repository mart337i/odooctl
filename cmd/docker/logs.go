package docker

import (
	"fmt"

	"github.com/egeskov/odooctl/internal/docker"
	"github.com/spf13/cobra"
)

var (
	flagFollow  bool
	flagLogTail int
)

var logsCmd = &cobra.Command{
	Use:   "logs [service]",
	Short: "View container logs",
	Long: `Shows logs from Docker containers. Defaults to the odoo service.

Examples:
  odooctl docker logs             # Last 100 lines of odoo logs
  odooctl docker logs -f          # Follow odoo logs
  odooctl docker logs --tail 50   # Last 50 lines
  odooctl docker logs db          # View database logs`,
	RunE: runLogs,
}

func init() {
	logsCmd.Flags().BoolVarP(&flagFollow, "follow", "f", false, "Follow log output")
	logsCmd.Flags().IntVar(&flagLogTail, "tail", 100, "Number of lines to show from the end of the logs")
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
	if flagLogTail > 0 {
		logArgs = append(logArgs, "--tail", fmt.Sprintf("%d", flagLogTail))
	}
	logArgs = append(logArgs, service)

	return docker.Compose(state, logArgs...)
}
