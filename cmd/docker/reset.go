package docker

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/mart337i/odooctl/internal/config"
	"github.com/mart337i/odooctl/internal/docker"
	"github.com/mart337i/odooctl/internal/output"
	"github.com/mart337i/odooctl/pkg/prompt"
	"github.com/spf13/cobra"
)

var (
	flagResetYes     bool
	flagResetVolumes bool
	flagResetFiles   bool
	flagResetJSON    bool
)

type resetReport struct {
	ContainersStopped bool   `json:"containers_stopped"`
	VolumesRemoved    bool   `json:"volumes_removed"`
	FilesRemoved      bool   `json:"files_removed"`
	DockerOutput      string `json:"docker_output,omitempty"`
	Warning           string `json:"warning,omitempty"`
}

var resetCmd = &cobra.Command{
	Use:          "reset",
	Short:        "Remove containers, optionally volumes and files",
	SilenceUsage: true,
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
	resetCmd.Flags().BoolVar(&flagResetJSON, "json", false, "Print JSON output")
}

func runReset(cmd *cobra.Command, args []string) error {
	state, err := loadState()
	if err != nil {
		return err
	}
	if flagResetJSON {
		return runResetJSON(state)
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
	dockerErr := docker.Compose(state, downArgs...)
	if dockerErr != nil {
		fmt.Printf("%s Warning: failed to stop containers", yellow("!"))
		if flagResetVolumes {
			fmt.Print("/remove volumes")
		}
		fmt.Printf(": %v\n", dockerErr)
	}

	if shouldKeepConfigAfterDockerCleanupError(dockerErr, flagResetVolumes, flagResetFiles) {
		return fmt.Errorf("docker cleanup failed; leaving config files in place so volumes can be removed later: %w", dockerErr)
	}

	// Remove environment directory if requested
	filesRemoved := false
	if flagResetFiles {
		dir, err := config.EnvironmentDir(state.ProjectName, state.Branch)
		if err != nil {
			return err
		}

		fmt.Printf("%s Removing config files...\n", yellow("→"))
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("failed to remove directory: %w", err)
		}
		if err := config.RemoveProjectLink(state.ProjectRoot); err != nil {
			return fmt.Errorf("failed to remove project link: %w", err)
		}
		filesRemoved = true
	}

	fmt.Println()
	if dockerErr != nil {
		msg := fmt.Sprintf("%s Docker cleanup failed", yellow("!"))
		if filesRemoved {
			msg += ", files removed"
		}
		fmt.Println(msg)
		if shouldReturnDockerCleanupError(dockerErr, filesRemoved) {
			return fmt.Errorf("docker cleanup failed: %w", dockerErr)
		}
		return nil
	}

	msg := fmt.Sprintf("%s Containers stopped", green("✓"))
	if flagResetVolumes {
		msg += ", volumes removed"
	}
	if filesRemoved {
		msg += ", files removed"
	}
	fmt.Println(msg)

	return nil
}

func runResetJSON(state *config.State) error {
	if (flagResetVolumes || flagResetFiles) && !flagResetYes {
		return fmt.Errorf("--json with destructive reset flags requires --force")
	}
	downArgs := []string{"down", "--remove-orphans"}
	if flagResetVolumes {
		downArgs = append(downArgs, "-v")
	}
	dockerOutput, dockerErr := docker.ComposeOutput(state, downArgs...)
	if shouldKeepConfigAfterDockerCleanupError(dockerErr, flagResetVolumes, flagResetFiles) {
		return fmt.Errorf("docker cleanup failed; leaving config files in place so volumes can be removed later: %w", dockerErr)
	}

	filesRemoved := false
	if flagResetFiles {
		dir, err := config.EnvironmentDir(state.ProjectName, state.Branch)
		if err != nil {
			return err
		}
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("failed to remove directory: %w", err)
		}
		if err := config.RemoveProjectLink(state.ProjectRoot); err != nil {
			return fmt.Errorf("failed to remove project link: %w", err)
		}
		filesRemoved = true
	}

	report := resetReport{
		ContainersStopped: dockerErr == nil,
		VolumesRemoved:    flagResetVolumes && dockerErr == nil,
		FilesRemoved:      filesRemoved,
		DockerOutput:      dockerOutput,
	}
	if dockerErr != nil {
		report.Warning = dockerErr.Error()
		if shouldReturnDockerCleanupError(dockerErr, filesRemoved) {
			return fmt.Errorf("docker cleanup failed: %w", dockerErr)
		}
	}
	return output.PrintJSON(report)
}

func shouldKeepConfigAfterDockerCleanupError(dockerErr error, removeVolumes, removeFiles bool) bool {
	return dockerErr != nil && removeVolumes && removeFiles
}

func shouldReturnDockerCleanupError(dockerErr error, filesRemoved bool) bool {
	return dockerErr != nil && !filesRemoved
}
