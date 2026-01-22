package docker

import (
	"fmt"
	"os"
	"strings"

	"github.com/egeskov/odooctl/internal/config"
	"github.com/egeskov/odooctl/internal/docker"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	flagRunBuild  bool
	flagRunInit   bool
	flagRunDetach bool
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Start the Docker development environment",
	Long: `Start the Docker development environment.

By default, just starts the containers. Use -i to initialize the database first.

Examples:
  odooctl docker run              # Start containers
  odooctl docker run -i           # Initialize database and start
  odooctl docker run --build      # Rebuild before starting`,
	RunE: runRun,
}

func init() {
	runCmd.Flags().BoolVarP(&flagRunBuild, "build", "b", false, "Rebuild containers before starting")
	runCmd.Flags().BoolVarP(&flagRunInit, "init", "i", false, "Initialize database before starting")
	runCmd.Flags().BoolVarP(&flagRunDetach, "detach", "d", true, "Run in background")
}

func runRun(cmd *cobra.Command, args []string) error {
	state, err := loadState()
	if err != nil {
		return err
	}

	green := color.New(color.FgGreen).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()

	// Initialize if requested
	if flagRunInit {
		fmt.Println("Initializing database...")

		// Build init command with modules
		modules := []string{"base", "web"}
		modules = append(modules, state.Modules...)

		initArgs := []string{
			"run", "--rm", "odoo",
			"odoo", "-c", "/etc/odoo/odoo.conf",
			"-d", getDBName(state),
			"-i", strings.Join(modules, ","),
		}

		// Use WithoutDemo from state (set during create)
		if state.WithoutDemo {
			initArgs = append(initArgs, "--without-demo=all")
		}
		initArgs = append(initArgs, "--stop-after-init")

		if err := docker.Compose(state, initArgs...); err != nil {
			return fmt.Errorf("failed to initialize: %w", err)
		}

		// Configure report.url parameter
		fmt.Println("Configuring report.url parameter...")
		sql := "INSERT INTO ir_config_parameter (key, value) VALUES ('report.url', 'http://odoo:8069') ON CONFLICT (key) DO UPDATE SET value = 'http://odoo:8069';"
		docker.Compose(state, "exec", "-T", "db", "psql", "-U", "odoo", "-d", getDBName(state), "-c", sql)

		fmt.Printf("%s Database initialized\n\n", green("✓"))
	}

	// Start main containers
	fmt.Println("Starting containers...")

	upArgs := []string{"up"}
	if flagRunDetach {
		upArgs = append(upArgs, "-d")
	}
	if flagRunBuild {
		upArgs = append(upArgs, "--build")
	}

	if err := docker.Compose(state, upArgs...); err != nil {
		return fmt.Errorf("failed to start containers: %w", err)
	}

	if flagRunDetach {
		fmt.Println()
		fmt.Printf("%s Containers started!\n\n", green("✓"))
		fmt.Printf("  Odoo:     %s\n", cyan(fmt.Sprintf("http://localhost:%d", state.Ports.Odoo)))
		fmt.Printf("  Mailhog:  %s\n", cyan(fmt.Sprintf("http://localhost:%d", state.Ports.Mailhog)))
		fmt.Println()
	}

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
