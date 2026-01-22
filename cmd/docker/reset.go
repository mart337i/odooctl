package docker

import (
	"fmt"
	"os"

	"github.com/egeskov/odooctl/internal/config"
	"github.com/egeskov/odooctl/internal/docker"
	"github.com/egeskov/odooctl/pkg/prompt"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var flagYes bool

var resetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Remove containers, volumes, and files",
	Long:  `Stops and removes all containers, volumes, and generated files for this project.`,
	RunE:  runReset,
}

func init() {
	resetCmd.Flags().BoolVarP(&flagYes, "yes", "y", false, "Skip confirmation prompt")
}

func runReset(cmd *cobra.Command, args []string) error {
	state, err := loadState()
	if err != nil {
		return err
	}

	// Confirm
	if !flagYes {
		confirmed, err := prompt.Confirm(
			fmt.Sprintf("This will delete all containers and data for %q. Continue?", state.ProjectName),
			false,
		)
		if err != nil || !confirmed {
			fmt.Println("Aborted.")
			return nil
		}
	}

	yellow := color.New(color.FgYellow).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()

	// Stop and remove containers
	fmt.Printf("%s Stopping containers...\n", yellow("→"))
	docker.Compose(state, "down", "-v", "--remove-orphans")

	// Remove project directory
	dir, err := config.ProjectDir(state.ProjectName)
	if err != nil {
		return err
	}

	fmt.Printf("%s Removing files...\n", yellow("→"))
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("failed to remove directory: %w", err)
	}

	fmt.Println()
	fmt.Printf("%s Project %q has been reset\n", green("✓"), state.ProjectName)

	return nil
}
