package docker

import (
	"fmt"
	"os"
	"time"

	"github.com/egeskov/odooctl/internal/config"
	"github.com/egeskov/odooctl/internal/docker"
	"github.com/egeskov/odooctl/internal/templates"
	"github.com/egeskov/odooctl/pkg/prompt"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	flagRunBuild    bool
	flagRunInit     bool
	flagRunDetach   bool
	flagRunNoPrompt bool
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
	runCmd.Flags().BoolVar(&flagRunNoPrompt, "no-prompt", false, "Skip interactive prompts (for CI/automation)")
}

func runRun(cmd *cobra.Command, args []string) error {
	state, err := loadState()
	if err != nil {
		return err
	}

	green := color.New(color.FgGreen).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()

	// Check for port conflicts
	available, conflicting := state.Ports.CheckPortsAvailable()
	if !available {
		fmt.Printf("%s Port conflict detected: %v\n", yellow("⚠️"), conflicting)
		fmt.Println("Regenerating configuration with available ports...")

		newPorts := config.FindAvailablePorts(state.OdooVersion)
		state.Ports = newPorts

		// Regenerate templates with new ports
		if err := templates.Render(state); err != nil {
			return fmt.Errorf("failed to regenerate templates: %w", err)
		}

		// Save updated state
		if err := state.Save(); err != nil {
			return fmt.Errorf("failed to save state: %w", err)
		}

		fmt.Printf("%s Files regenerated with new ports\n", green("✓"))
	}

	// Prompt for build if never done before
	if state.BuiltAt == nil && !flagRunBuild && !flagRunNoPrompt {
		shouldBuild, err := prompt.Confirm("Docker images have never been built. Build now?", true)
		if err != nil {
			return err
		}
		if shouldBuild {
			flagRunBuild = true
		} else {
			fmt.Printf("%s Skipping build. Containers may fail if images don't exist.\n", yellow("⚠️"))
		}
	}

	// Prompt for init if never done before
	if state.InitializedAt == nil && !flagRunInit && !flagRunNoPrompt {
		shouldInit, err := prompt.Confirm("Database has never been initialized. Initialize now?", true)
		if err != nil {
			return err
		}
		if shouldInit {
			flagRunInit = true
		} else {
			fmt.Printf("%s Skipping initialization. Odoo may not start correctly.\n", yellow("⚠️"))
		}
	}

	fmt.Println("Starting containers...")
	// Start main containers
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

	// Track that build has been done
	if flagRunBuild && state.BuiltAt == nil {
		now := time.Now()
		state.BuiltAt = &now
		if err := state.Save(); err != nil {
			return fmt.Errorf("failed to save state: %w", err)
		}
	}

	// Initialize if requested
	if flagRunInit {
		fmt.Println("Initializing database...")

		// Use the odoo-init service defined in docker-compose (activated via the
		// "init" profile). Its command is rendered by the template and already
		// handles the demo-data flag correctly for every Odoo version.
		if err := docker.Compose(state, "--profile", "init", "up", "odoo-init"); err != nil {
			return fmt.Errorf("failed to initialize: %w", err)
		}

		// Configure report.url parameter
		fmt.Println("Configuring report.url parameter...")
		sql := "INSERT INTO ir_config_parameter (key, value) VALUES ('report.url', 'http://odoo:8069') ON CONFLICT (key) DO UPDATE SET value = 'http://odoo:8069';"
		if err := docker.Compose(state, "exec", "-T", "db", "psql", "-U", "odoo", "-d", getDBName(state), "-c", sql); err != nil {
			fmt.Printf("%s Warning: failed to configure report.url: %v\n", yellow("⚠️"), err)
		}

		// Track that initialization has been done
		if state.InitializedAt == nil {
			now := time.Now()
			state.InitializedAt = &now
			if err := state.Save(); err != nil {
				return fmt.Errorf("failed to save state: %w", err)
			}
		}

		fmt.Printf("%s Database initialized\n\n", green("✓"))
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
