package docker

import (
	"fmt"
	"os"

	"github.com/egeskov/odooctl/internal/config"
	"github.com/egeskov/odooctl/internal/docker"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var flagBuild bool

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Start the Docker development environment",
	Long:  `Initializes the database (if needed) and starts all containers.`,
	RunE:  runRun,
}

func init() {
	runCmd.Flags().BoolVarP(&flagBuild, "build", "b", false, "Rebuild containers before starting")
}

func runRun(cmd *cobra.Command, args []string) error {
	state, err := loadState()
	if err != nil {
		return err
	}

	green := color.New(color.FgGreen).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()

	// Check if we need to initialize (first run)
	if !docker.IsRunning(state) {
		fmt.Println("Initializing database...")

		initArgs := []string{"--profile", "init", "up"}
		if flagBuild {
			initArgs = append(initArgs, "--build")
		}
		initArgs = append(initArgs, "odoo-init")

		if err := docker.Compose(state, initArgs...); err != nil {
			return fmt.Errorf("failed to initialize: %w", err)
		}

		fmt.Printf("%s Database initialized\n\n", green("✓"))
	}

	// Start main containers
	fmt.Println("Starting containers...")

	upArgs := []string{"up", "-d"}
	if flagBuild {
		upArgs = append(upArgs, "--build")
	}

	if err := docker.Compose(state, upArgs...); err != nil {
		return fmt.Errorf("failed to start containers: %w", err)
	}

	fmt.Println()
	fmt.Printf("%s Containers started!\n\n", green("✓"))
	fmt.Printf("  Odoo:     %s\n", cyan(fmt.Sprintf("http://localhost:%d", state.Ports.Odoo)))
	fmt.Printf("  Mailhog:  %s\n", cyan(fmt.Sprintf("http://localhost:%d", state.Ports.Mailhog)))
	fmt.Println()

	return nil
}

func loadState() (*config.State, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	state, err := config.LoadFromDir(cwd)
	if err != nil {
		return nil, fmt.Errorf("no Docker environment found. Run 'odooctl docker create' first")
	}

	return state, nil
}
