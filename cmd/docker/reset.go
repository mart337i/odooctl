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

var (
	flagResetYes     bool
	flagResetVolumes bool
	flagResetFiles   bool
)

var resetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Remove containers, optionally volumes and files",
	Long: `Stop and remove containers for this project.

By default, only stops containers. Use flags to also remove data:
  -v  Remove Docker volumes (database, filestore)
  -c  Remove config files (~/.odooctl/{project}/)

Examples:
  odooctl docker reset           # Stop containers only
  odooctl docker reset -v        # Stop containers and remove volumes
  odooctl docker reset -c        # Stop containers and remove config files
  odooctl docker reset -v -c     # Full cleanup (containers, volumes, files)
  odooctl docker reset -v -c -f  # Full cleanup without confirmation`,
	RunE: runReset,
}

func init() {
	resetCmd.Flags().BoolVarP(&flagResetYes, "force", "f", false, "Skip confirmation prompt")
	resetCmd.Flags().BoolVarP(&flagResetVolumes, "volumes", "v", false, "Remove Docker volumes (database, filestore)")
	resetCmd.Flags().BoolVarP(&flagResetFiles, "files", "c", false, "Remove config files")
}

func runReset(cmd *cobra.Command, args []string) error {
	state, err := loadState()
	if err != nil {
		return err
	}

	yellow := color.New(color.FgYellow).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()

	// Confirm if removing data
	if (flagResetVolumes || flagResetFiles) && !flagResetYes {
		msg := "This will delete containers"
		if flagResetVolumes {
			msg += ", volumes (database)"
		}
		if flagResetFiles {
			msg += ", config files"
		}
		msg += fmt.Sprintf(" for %q. Continue?", state.ProjectName)

		confirmed, err := prompt.Confirm(msg, false)
		if err != nil || !confirmed {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// Stop and remove containers
	fmt.Printf("%s Stopping containers...\n", yellow("→"))

	downArgs := []string{"down", "--remove-orphans"}
	if flagResetVolumes {
		downArgs = append(downArgs, "-v")
	}
	docker.Compose(state, downArgs...)

	// Remove environment directory if requested
	if flagResetFiles {
		dir, err := config.EnvironmentDir(state.ProjectName, state.Branch)
		if err != nil {
			return err
		}

		fmt.Printf("%s Removing config files...\n", yellow("→"))
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("failed to remove directory: %w", err)
		}
	}

	fmt.Println()
	msg := fmt.Sprintf("%s Containers stopped", green("✓"))
	if flagResetVolumes {
		msg += ", volumes removed"
	}
	if flagResetFiles {
		msg += ", files removed"
	}
	fmt.Println(msg)

	return nil
}
