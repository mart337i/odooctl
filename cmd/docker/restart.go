package docker

import (
	"fmt"

	"github.com/fatih/color"
	dockerlib "github.com/mart337i/odooctl/internal/docker"
	"github.com/mart337i/odooctl/internal/output"
	"github.com/spf13/cobra"
)

var flagRestartJSON bool

type restartReport struct {
	Services []string `json:"services"`
	Output   string   `json:"output,omitempty"`
}

var restartCmd = &cobra.Command{
	Use:          "restart [service...]",
	Short:        "Restart one or more services",
	SilenceUsage: true,
	Long: `Restart services in the current environment. Defaults to restarting only
the Odoo service, which is usually what a developer needs after Python changes.`,
	Args: cobra.ArbitraryArgs,
	RunE: runRestart,
}

func init() {
	restartCmd.Flags().BoolVar(&flagRestartJSON, "json", false, "Print JSON output")
}

func runRestart(cmd *cobra.Command, args []string) error {
	state, err := loadState()
	if err != nil {
		return err
	}
	services := args
	if len(services) == 0 {
		services = []string{"odoo"}
	}
	composeArgs := append([]string{"restart"}, services...)
	if flagRestartJSON {
		text, err := dockerlib.ComposeOutput(state, composeArgs...)
		if err != nil {
			return fmt.Errorf("failed to restart services: %w", err)
		}
		return output.PrintJSON(restartReport{Services: services, Output: text})
	}
	fmt.Printf("Restarting %s...\n", color.CyanString(joinServices(services)))
	if err := dockerlib.Compose(state, composeArgs...); err != nil {
		return fmt.Errorf("failed to restart services: %w", err)
	}
	fmt.Printf("%s Restarted %s\n", color.GreenString("✓"), joinServices(services))
	return nil
}

func joinServices(services []string) string {
	if len(services) == 1 {
		return services[0]
	}
	return fmt.Sprintf("%v", services)
}
